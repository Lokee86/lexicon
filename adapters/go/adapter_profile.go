package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type adapterProfilePhase struct {
	Name       string `json:"name"`
	DurationNS int64  `json:"duration_ns"`
}

type adapterProfileReport struct {
	Version int                   `json:"version"`
	Phases  []adapterProfilePhase `json:"phases"`
	Counts  map[string]int64      `json:"counts"`
}

type adapterProfiler struct {
	path   string
	phases []adapterProfilePhase
	counts map[string]int64
}

var adapterMetrics = newAdapterProfiler()

func newAdapterProfiler() *adapterProfiler {
	return &adapterProfiler{path: os.Getenv("LEXICON_ADAPTER_PROFILE"), counts: make(map[string]int64)}
}

func (p *adapterProfiler) measure(name string) func() {
	if p == nil || p.path == "" {
		return func() {}
	}
	started := time.Now()
	return func() {
		p.phases = append(p.phases, adapterProfilePhase{Name: name, DurationNS: time.Since(started).Nanoseconds()})
	}
}

func (p *adapterProfiler) set(name string, value int64) {
	if p != nil && p.path != "" {
		p.counts[name] = value
	}
}

func (p *adapterProfiler) write() error {
	if p == nil || p.path == "" {
		return nil
	}
	data, err := json.MarshalIndent(adapterProfileReport{Version: 1, Phases: p.phases, Counts: p.counts}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p.path, append(data, '\n'), 0o644)
}
