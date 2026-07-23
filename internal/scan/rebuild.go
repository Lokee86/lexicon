package scan

import (
	"context"
	"fmt"
	"path/filepath"

	languageRegistry "github.com/Lokee86/lexicon/internal/languages"
	"github.com/Lokee86/lexicon/internal/lock"
)

func (s *Scanner) Rebuild(ctx context.Context, languages []string) (report Report, err error) {
	s.beginProfile("rebuild")
	s.Profile.Set("requested_languages", int64(len(languages)))
	defer s.finishProfile(&err)
	report, err = s.rebuild(ctx, languages)
	return s.notifyConsumers(ctx, report, err)
}

func (s *Scanner) rebuild(ctx context.Context, languages []string) (Report, error) {
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
	_, err = s.pruneDisabledLibraries()
	finishPrune()
	if err != nil {
		return Report{}, err
	}
	finishMirror := s.Profile.Measure("mirror.sync", "", "full")
	err = s.Mirror.SyncAll(s.Repository)
	finishMirror()
	if err != nil {
		return Report{}, err
	}
	if err := s.Git.StageSource(); err != nil {
		return Report{}, err
	}
	finishChanges := s.Profile.Measure("changes.detect", "", "")
	changes, err := s.Git.SourceChanges()
	finishChanges()
	if err != nil {
		return Report{}, err
	}
	s.Profile.Set("files_changed", int64(len(changes)))
	if len(languages) == 0 {
		languages, err = languagesInTree(filepath.Join(s.StateRoot, "source"))
		if err != nil {
			return Report{}, err
		}
		languages = selectedLanguages(languages, s.languageEnabled)
	} else {
		languages, err = s.validateRebuildLanguages(languages)
		if err != nil {
			return Report{}, err
		}
	}
	s.Profile.Set("analysis_plans", int64(len(languages)))
	s.Profile.Set("analysis_full_plans", int64(len(languages)))
	plans := make([]analysisPlan, 0, len(languages))
	for _, language := range languages {
		plans = append(plans, analysisPlan{Language: language, Full: true})
	}
	if err := s.analyzePlans(ctx, plans); err != nil {
		return Report{}, err
	}
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

func (s *Scanner) validateRebuildLanguages(languages []string) ([]string, error) {
	supported := make(map[string]struct{}, len(languageRegistry.Supported()))
	for _, language := range languageRegistry.Supported() {
		supported[language] = struct{}{}
	}
	for _, language := range languages {
		if _, ok := supported[language]; !ok {
			return nil, fmt.Errorf("unsupported Lexicon language %q", language)
		}
		if !s.languageEnabled(language) {
			return nil, fmt.Errorf("Lexicon language %q is disabled", language)
		}
	}
	return uniqueSorted(languages), nil
}
