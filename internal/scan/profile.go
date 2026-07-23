package scan

import (
	"os"

	"github.com/Lokee86/lexicon/internal/profile"
)

func (s *Scanner) beginProfile(operation string) {
	path := s.ProfilePath
	if path == "" {
		path = os.Getenv("LEXICON_PROFILE")
	}
	s.Profile = profile.New(path, operation)
	s.Store.Profile = s.Profile
}

func (s *Scanner) finishProfile(operationErr *error) {
	if s.Profile == nil {
		return
	}
	profileErr := s.Profile.Write(*operationErr)
	s.Profile = nil
	s.Store.Profile = nil
	if *operationErr == nil {
		*operationErr = profileErr
	}
}

func analysisMode(full bool) string {
	if full {
		return "full"
	}
	return "incremental"
}
