package scan

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/Lokee86/lexicon/internal/adapters"
	"github.com/Lokee86/lexicon/internal/config"
	"github.com/Lokee86/lexicon/internal/lock"
	"github.com/Lokee86/lexicon/internal/objectstore"
	"github.com/Lokee86/lexicon/internal/profile"
	"github.com/Lokee86/lexicon/internal/state"
)

func Initialize(ctx context.Context, repository, adapterRoot string, output io.Writer) (*Scanner, Report, error) {
	return initialize(ctx, repository, adapterRoot, nil, false, output)
}

func InitializeWithLanguages(
	ctx context.Context,
	repository, adapterRoot string,
	enabledLanguages []string,
	output io.Writer,
) (*Scanner, Report, error) {
	return initialize(ctx, repository, adapterRoot, enabledLanguages, true, output)
}

func initialize(
	ctx context.Context,
	repository, adapterRoot string,
	enabledLanguages []string,
	explicitSelection bool,
	output io.Writer,
) (resultScanner *Scanner, resultReport Report, resultErr error) {
	metrics := profile.New(os.Getenv("LEXICON_PROFILE"), "init")
	defer func() {
		if profileErr := metrics.Write(resultErr); resultErr == nil {
			resultErr = profileErr
		}
	}()
	finishResolve := metrics.Measure("repository.resolve", "", "")
	absolute, err := filepath.Abs(repository)
	finishResolve()
	if err != nil {
		return nil, Report{}, err
	}
	lexiconRoot := config.StateRoot(absolute)
	guard, err := lock.Acquire(lexiconRoot)
	if err != nil {
		return nil, Report{}, err
	}
	defer guard.Close()
	finishConfig := metrics.Measure("config.save", "", "")
	if explicitSelection {
		err = config.SaveWithEnabledLanguages(absolute, adapterRoot, enabledLanguages)
	} else {
		err = config.Save(absolute, adapterRoot)
	}
	finishConfig()
	if err != nil {
		return nil, Report{}, err
	}
	stateRoot := filepath.Join(lexiconRoot, "repo")
	finishEnsure := metrics.Measure("state.ensure", "", "")
	gitRepository, err := state.Ensure(stateRoot)
	finishEnsure()
	if err != nil {
		return nil, Report{}, err
	}
	scanner := New(absolute, stateRoot, gitRepository, adapters.Runner{Root: adapterRoot}, output)
	scanner.Profile = metrics
	scanner.Store.Profile = metrics
	configuration, err := config.Load(absolute)
	if err != nil {
		return nil, Report{}, err
	}
	scanner.EnabledLanguages = configuration.EnabledLanguages
	finishMirror := metrics.Measure("mirror.sync", "", "full")
	err = scanner.Mirror.SyncAll(absolute)
	finishMirror()
	if err != nil {
		return nil, Report{}, err
	}
	finishLanguages := metrics.Measure("languages.discover", "", "")
	languages, err := languagesInTree(filepath.Join(stateRoot, "source"))
	finishLanguages()
	if err != nil {
		return nil, Report{}, err
	}
	languages = selectedLanguages(languages, scanner.languageEnabled)
	metrics.Set("languages_discovered", int64(len(languages)))
	metrics.Set("analysis_plans", int64(len(languages)))
	metrics.Set("analysis_full_plans", int64(len(languages)))
	finishPrune := metrics.Measure("library.prune_disabled", "", "")
	_, err = scanner.pruneDisabledLibraries()
	finishPrune()
	if err != nil {
		return nil, Report{}, err
	}
	if err := scanner.analyzeFull(ctx, languages); err != nil {
		return nil, Report{}, err
	}
	finishStageAll := metrics.Measure("state.stage_all", "", "")
	err = gitRepository.StageAll()
	finishStageAll()
	if err != nil {
		return nil, Report{}, err
	}
	finishCommit := metrics.Measure("state.commit", "", "")
	err = gitRepository.CommitState()
	finishCommit()
	if err != nil {
		return nil, Report{}, err
	}
	snapshotID, err := scanner.publishSnapshot()
	if err != nil {
		return nil, Report{}, err
	}
	return scanner, Report{Languages: languages, SnapshotID: snapshotID}, nil
}

func Open(repository string, output io.Writer) (*Scanner, error) {
	absolute, err := filepath.Abs(repository)
	if err != nil {
		return nil, err
	}
	configuration, err := config.Load(absolute)
	if err != nil {
		return nil, err
	}
	stateRoot := filepath.Join(config.StateRoot(absolute), "repo")
	gitRepository, err := state.Open(stateRoot)
	if err != nil {
		return nil, err
	}
	scanner := New(absolute, stateRoot, gitRepository, adapters.Runner{Root: configuration.AdapterRoot}, output)
	scanner.EnabledLanguages = configuration.EnabledLanguages
	return scanner, nil
}

func New(repository, stateRoot string, gitRepository *state.Repository, analyzer adapters.Analyzer, output io.Writer) *Scanner {
	return &Scanner{
		Repository:  repository,
		StateRoot:   stateRoot,
		AdapterRoot: adapterRoot(analyzer),
		Git:         gitRepository,
		Mirror:      state.Mirror{Root: filepath.Join(stateRoot, "source")},
		Analyzer:    analyzer,
		Store:       objectstore.Store{Root: config.StateRoot(repository)},
		Output:      output,
		ProfilePath: os.Getenv("LEXICON_PROFILE"),
	}
}

func adapterRoot(analyzer adapters.Analyzer) string {
	switch value := analyzer.(type) {
	case adapters.Runner:
		return value.Root
	case *adapters.Runner:
		if value != nil {
			return value.Root
		}
	}
	return ""
}
