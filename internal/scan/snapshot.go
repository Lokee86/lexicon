package scan

import (
	"errors"
	"sort"

	"github.com/Lokee86/lexicon/internal/adapters"
	"github.com/Lokee86/lexicon/internal/config"
	"github.com/Lokee86/lexicon/internal/objectstore"
)

func (s *Scanner) ensureSnapshot() (string, error) {
	finishHead := s.Profile.Measure("snapshot.read_head", "", "")
	head, err := s.Git.Head()
	finishHead()
	if err != nil {
		return "", err
	}
	finishCurrent := s.Profile.Measure("snapshot.load_current", "", "")
	id, manifest, err := s.Store.Current()
	finishCurrent()
	if err == nil && manifest.StateCommit == head {
		return id, nil
	}
	return s.publishSnapshot()
}

func (s *Scanner) publishSnapshot() (string, error) {
	finishHead := s.Profile.Measure("snapshot.read_head", "", "")
	head, err := s.Git.Head()
	finishHead()
	if err != nil {
		return "", err
	}
	finishBuild := s.Profile.Measure("snapshot.build_manifest", "", "")
	manifest, err := s.Store.BuildManifest(s.StateRoot, head, config.AnalysisID(), s.AdapterRoot)
	finishBuild()
	if err != nil {
		return "", err
	}
	s.Profile.Set("snapshot_languages", int64(len(manifest.Languages)))
	finishPublish := s.Profile.Measure("snapshot.publish", "", "")
	id, err := s.Store.Publish(manifest)
	finishPublish()
	return id, err
}

func (s *Scanner) adapterDriftLanguages() ([]string, error) {
	if s.AdapterRoot == "" {
		return nil, nil
	}
	_, manifest, err := s.Store.Current()
	if errors.Is(err, objectstore.ErrNoCurrentSnapshot) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	drift := make([]string, 0)
	for _, language := range manifest.Languages {
		fingerprint, err := adapters.Fingerprint(s.AdapterRoot, language.Language)
		if err != nil {
			return nil, err
		}
		if fingerprint != language.AdapterFingerprint {
			drift = append(drift, language.Language)
		}
	}
	sort.Strings(drift)
	return drift, nil
}
