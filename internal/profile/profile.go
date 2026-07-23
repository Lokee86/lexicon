package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const Version = 1

type Phase struct {
	Name       string `json:"name"`
	Language   string `json:"language,omitempty"`
	Mode       string `json:"mode,omitempty"`
	DurationNS int64  `json:"duration_ns"`
}

type Report struct {
	Version    int              `json:"version"`
	Operation  string           `json:"operation"`
	StartedAt  time.Time        `json:"started_at"`
	DurationNS int64            `json:"duration_ns"`
	Status     string           `json:"status"`
	Error      string           `json:"error,omitempty"`
	Phases     []Phase          `json:"phases"`
	Counts     map[string]int64 `json:"counts"`
}

type Recorder struct {
	mu        sync.Mutex
	path      string
	operation string
	started   time.Time
	phases    []Phase
	counts    map[string]int64
}

func New(path, operation string) *Recorder {
	if path == "" {
		return nil
	}
	return &Recorder{
		path: path, operation: operation, started: time.Now(),
		counts: make(map[string]int64),
	}
}

func (r *Recorder) Measure(name, language, mode string) func() {
	if r == nil {
		return func() {}
	}
	started := time.Now()
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.phases = append(r.phases, Phase{
			Name: name, Language: language, Mode: mode,
			DurationNS: time.Since(started).Nanoseconds(),
		})
	}
}

func (r *Recorder) Add(name string, value int64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counts[name] += value
}

func (r *Recorder) Set(name string, value int64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counts[name] = value
}

func (r *Recorder) MergeAdapter(path, language, mode string) error {
	if r == nil || path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s adapter profile: %w", language, err)
	}
	var report struct {
		Version int              `json:"version"`
		Phases  []Phase          `json:"phases"`
		Counts  map[string]int64 `json:"counts"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return fmt.Errorf("decode %s adapter profile: %w", language, err)
	}
	if report.Version != Version {
		return fmt.Errorf("unsupported %s adapter profile version %d", language, report.Version)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, phase := range report.Phases {
		phase.Language = language
		if phase.Mode == "" {
			phase.Mode = mode
		}
		phase.Name = "adapter." + phase.Name
		r.phases = append(r.phases, phase)
	}
	for name, value := range report.Counts {
		r.counts["adapter_"+name] += value
		r.counts["adapter_"+language+"_"+name] += value
	}
	return nil
}

func (r *Recorder) Write(operationErr error) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	report := Report{
		Version: Version, Operation: r.operation, StartedAt: r.started.UTC(),
		DurationNS: time.Since(r.started).Nanoseconds(), Status: "ok",
		Phases: append([]Phase(nil), r.phases...), Counts: cloneCounts(r.counts),
	}
	r.mu.Unlock()
	if operationErr != nil {
		report.Status = "error"
		report.Error = operationErr.Error()
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Lexicon profile: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return fmt.Errorf("create Lexicon profile directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(r.path), ".lexicon-profile-*")
	if err != nil {
		return fmt.Errorf("create Lexicon profile: %w", err)
	}
	temporaryPath := temporary.Name()
	cleanup := func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}
	if _, err := temporary.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write Lexicon profile: %w", err)
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("close Lexicon profile: %w", err)
	}
	if err := os.Rename(temporaryPath, r.path); err != nil {
		if removeErr := os.Remove(r.path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			_ = os.Remove(temporaryPath)
			return fmt.Errorf("replace Lexicon profile: %w", removeErr)
		}
		if err := os.Rename(temporaryPath, r.path); err != nil {
			_ = os.Remove(temporaryPath)
			return fmt.Errorf("replace Lexicon profile: %w", err)
		}
	}
	return nil
}

func cloneCounts(source map[string]int64) map[string]int64 {
	result := make(map[string]int64, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
