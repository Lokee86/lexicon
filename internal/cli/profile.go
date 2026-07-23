package cli

import "os"

func configureProfile(path string) func() {
	if path == "" {
		return func() {}
	}
	previous, existed := os.LookupEnv("LEXICON_PROFILE")
	_ = os.Setenv("LEXICON_PROFILE", path)
	return func() {
		if existed {
			_ = os.Setenv("LEXICON_PROFILE", previous)
		} else {
			_ = os.Unsetenv("LEXICON_PROFILE")
		}
	}
}
