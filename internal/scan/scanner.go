package scan

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Lokee86/lexicon/internal/adapters"
	"github.com/Lokee86/lexicon/internal/config"
	"github.com/Lokee86/lexicon/internal/consumer"
	"github.com/Lokee86/lexicon/internal/lock"
	"github.com/Lokee86/lexicon/internal/objectstore"
	"github.com/Lokee86/lexicon/internal/profile"
	"github.com/Lokee86/lexicon/internal/state"
)

type Scanner struct {
	Repository       string
	StateRoot        string
	AdapterRoot      string
	EnabledLanguages []string
	Git              *state.Repository
	Mirror           state.Mirror
	Analyzer         adapters.Analyzer
	Store            objectstore.Store
	Output           io.Writer
	ProfilePath      string
	Profile          *profile.Recorder
}

type Report struct {
	Changed    []state.Change
	Languages  []string
	SnapshotID string
}

func (s *Scanner) Scan(ctx context.Context) (report Report, err error) {
	s.beginProfile("scan")
	defer s.finishProfile(&err)
	report, err = s.scan(ctx, func() error {
		finish := s.Profile.Measure("mirror.sync", "", "full")
		defer finish()
		return s.Mirror.SyncAll(s.Repository)
	})
	return s.notifyConsumers(ctx, report, err)
}

func (s *Scanner) ScanPaths(ctx context.Context, paths []string) (report Report, err error) {
	s.beginProfile("scan_paths")
	s.Profile.Set("requested_paths", int64(len(paths)))
	defer s.finishProfile(&err)
	report, err = s.scan(ctx, func() error {
		finish := s.Profile.Measure("mirror.sync", "", "paths")
		defer finish()
		return s.Mirror.SyncPaths(s.Repository, paths)
	})
	return s.notifyConsumers(ctx, report, err)
}

func (s *Scanner) notifyConsumers(ctx context.Context, report Report, scanErr error) (Report, error) {
	if scanErr != nil {
		return report, scanErr
	}
	finish := s.Profile.Measure("consumers.run", "", "")
	defer finish()
	if err := consumer.Run(ctx, s.Repository, s.Store.Root, report.SnapshotID, s.Output); err != nil {
		return report, err
	}
	return report, nil
}

func (s *Scanner) scan(ctx context.Context, synchronize func() error) (Report, error) {
	finishLock := s.Profile.Measure("lock.acquire", "", "")
	guard, err := lock.Acquire(s.Store.Root)
	finishLock()
	if err != nil {
		return Report{}, err
	}
	defer guard.Close()
	finishReset := s.Profile.Measure("state.reset_index", "", "")
	err = s.Git.ResetIndex()
	finishReset()
	if err != nil {
		return Report{}, err
	}
	finishRestore := s.Profile.Measure("state.restore_library", "", "")
	err = s.Git.RestoreLibrary()
	finishRestore()
	if err != nil {
		return Report{}, err
	}
	finishPrune := s.Profile.Measure("library.prune_disabled", "", "")
	pruned, err := s.pruneDisabledLibraries()
	finishPrune()
	if err != nil {
		return Report{}, err
	}
	if err := synchronize(); err != nil {
		return Report{}, err
	}
	finishStageSource := s.Profile.Measure("state.stage_source", "", "")
	err = s.Git.StageSource()
	finishStageSource()
	if err != nil {
		return Report{}, err
	}
	finishChanges := s.Profile.Measure("changes.detect", "", "")
	changes, err := s.Git.SourceChanges()
	finishChanges()
	if err != nil {
		return Report{}, err
	}
	s.Profile.Set("files_changed", int64(len(changes)))
	finishDrift := s.Profile.Measure("library.detect_drift", "", "")
	drift, err := libraryDriftLanguagesFor(s.StateRoot, s.languageEnabled)
	finishDrift()
	if err != nil {
		return Report{}, err
	}
	finishAdapterDrift := s.Profile.Measure("adapters.detect_drift", "", "")
	adapterDrift, err := s.adapterDriftLanguages()
	finishAdapterDrift()
	if err != nil {
		return Report{}, err
	}
	drift = mergeLanguages(drift, adapterDrift)
	s.Profile.Set("languages_drifted", int64(len(drift)))
	finishPlan := s.Profile.Measure("analysis.plan", "", "")
	plans, err := s.plansFor(changes, drift)
	finishPlan()
	if err != nil {
		return Report{}, err
	}
	s.Profile.Set("analysis_plans", int64(len(plans)))
	for _, plan := range plans {
		s.Profile.Add("analysis_changed_files", int64(len(plan.ChangedFiles)))
		s.Profile.Add("analysis_removed_files", int64(len(plan.RemovedFiles)))
		s.Profile.Add("analysis_context_files", int64(len(plan.ContextFiles)))
		if plan.Full {
			s.Profile.Add("analysis_full_plans", 1)
		} else {
			s.Profile.Add("analysis_incremental_plans", 1)
		}
	}
	if len(changes) == 0 && len(plans) == 0 {
		if pruned {
			if err := s.Git.StageAll(); err != nil {
				return Report{}, err
			}
			if err := s.Git.CommitState(); err != nil {
				return Report{}, err
			}
			snapshotID, err := s.publishSnapshot()
			return Report{SnapshotID: snapshotID}, err
		}
		snapshotID, err := s.ensureSnapshot()
		return Report{SnapshotID: snapshotID}, err
	}
	if err := s.analyzePlans(ctx, plans); err != nil {
		return Report{}, err
	}
	languages := planLanguages(plans)
	finishStageAll := s.Profile.Measure("state.stage_all", "", "")
	err = s.Git.StageAll()
	finishStageAll()
	if err != nil {
		return Report{}, err
	}
	finishCommit := s.Profile.Measure("state.commit", "", "")
	err = s.Git.CommitState()
	finishCommit()
	if err != nil {
		return Report{}, err
	}
	snapshotID, err := s.publishSnapshot()
	if err != nil {
		return Report{}, err
	}
	return Report{Changed: changes, Languages: languages, SnapshotID: snapshotID}, nil
}

func (s *Scanner) languageEnabled(language string) bool {
	return config.Config{EnabledLanguages: s.EnabledLanguages}.LanguageEnabled(language)
}

func (s *Scanner) pruneDisabledLibraries() (bool, error) {
	libraryRoot := filepath.Join(s.StateRoot, "library")
	entries, err := os.ReadDir(libraryRoot)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	removed := false
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		language := strings.TrimSuffix(entry.Name(), ".jsonl")
		if s.languageEnabled(language) {
			continue
		}
		if err := os.Remove(filepath.Join(libraryRoot, entry.Name())); err != nil && !os.IsNotExist(err) {
			return false, err
		}
		removed = true
	}
	return removed, nil
}
