package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/shigabutdinoff/go-godot-kit/cmd/staticlint/internal/checks"
)

func TestOSExit(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), OSExitAnalyzer, "a", "b", "c", "d")
}

func TestAnalyzers(t *testing.T) {
	t.Parallel()
	as, err := analyzers("")
	require.NoError(t, err)
	names := map[string]bool{}
	for _, a := range as {
		assert.False(t, names[a.Name], "analyzer %s is registered twice", a.Name)
		names[a.Name] = true
	}
	for _, want := range []string{
		"shadow", "slog", "sigchanyzer", "nilness", "hostport",
		"SA1000", "S1000", "ST1001", "QF1001",
		"errcheck", "ineffassign",
		"osexit",
	} {
		assert.True(t, names[want], "analyzer %s is missing", want)
	}
}

func TestConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	exe := filepath.Join(dir, "staticlint")

	data, err := config(exe)
	require.NoError(t, err)
	assert.Equal(t, checks.Default, data, "no config.json next to the executable: the built-in one")

	require.NoError(t, os.WriteFile(filepath.Join(dir, configName), []byte(`{"staticcheck": ["SA5000"]}`), 0o600))
	as, err := analyzers(exe)
	require.NoError(t, err)
	var sa []string
	for _, a := range as {
		if strings.HasPrefix(a.Name, "SA") || strings.HasPrefix(a.Name, "ST") {
			sa = append(sa, a.Name)
		}
	}
	assert.Equal(t, []string{"SA5000"}, sa, "config.json next to the executable replaces the built-in one")

	require.NoError(t, os.WriteFile(filepath.Join(dir, configName), []byte(`{"staticcheck": ["XX1"]}`), 0o600))
	_, err = analyzers(exe)
	assert.ErrorContains(t, err, "XX1")

	data, err = config("")
	require.NoError(t, err)
	assert.Equal(t, checks.Default, data)
}

func TestMachineOutput(t *testing.T) {
	t.Parallel()
	assert.False(t, machineOutput([]string{"./..."}))
	for _, args := range [][]string{{"-flags"}, {"-V=full"}, {"/tmp/vet.cfg"}, {"-json", "./..."}} {
		assert.True(t, machineOutput(args), "%v", args)
	}
}
