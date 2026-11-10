// Package checks reads the list of staticcheck checks for staticlint and its doc generator from config.json.
package checks

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"honnef.co/go/tools/analysis/lint"
	"honnef.co/go/tools/quickfix"
	"honnef.co/go/tools/simple"
	"honnef.co/go/tools/staticcheck"
	"honnef.co/go/tools/stylecheck"
)

// Default is the built-in config.json: all SA, S, ST and QF checks except ST1000, whose package comment
// requirement is covered by revive in golangci-lint.
//
//go:embed config.json
var Default []byte

// Config is the content of config.json.
type Config struct {
	// Staticcheck lists check names, such as SA5000; "all" enables every SA, S, ST and QF check and "-NAME"
	// disables one, later entries taking precedence.
	Staticcheck []string `json:"staticcheck"`
}

// Staticcheck returns the staticcheck checks that config.json data enables, in staticcheck order.
// It returns an error for invalid JSON or an unknown check name.
func Staticcheck(data []byte) ([]*lint.Analyzer, error) {
	var cfg Config
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("config.json: %w", err)
	}
	var all []*lint.Analyzer
	known := map[string]bool{}
	for _, group := range [][]*lint.Analyzer{staticcheck.Analyzers, simple.Analyzers, stylecheck.Analyzers, quickfix.Analyzers} {
		for _, a := range group {
			all = append(all, a)
			known[a.Analyzer.Name] = true
		}
	}
	enabled := map[string]bool{}
	for _, c := range cfg.Staticcheck {
		name, off := strings.CutPrefix(c, "-")
		switch {
		case name == "all":
			for n := range known {
				enabled[n] = !off
			}
		case known[name]:
			enabled[name] = !off
		default:
			return nil, fmt.Errorf("config.json: unknown staticcheck check %q", c)
		}
	}
	var as []*lint.Analyzer
	for _, a := range all {
		if enabled[a.Analyzer.Name] {
			as = append(as, a)
		}
	}
	return as, nil
}
