package main

import (
	"bytes"
	"errors"
	"flag"
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStart(t *testing.T) {
	t.Parallel()
	require.Error(t, start(filepath.Join(t.TempDir(), "missing")), "missing directory is a load error")
}

func TestParseArgs(t *testing.T) {
	t.Parallel()
	root, err := parseArgs(nil, io.Discard)
	require.NoError(t, err)
	assert.Equal(t, ".", root)

	root, err = parseArgs([]string{"module"}, io.Discard)
	require.NoError(t, err)
	assert.Equal(t, "module", root)

	var help bytes.Buffer
	_, err = parseArgs([]string{"-h"}, &help)
	require.ErrorIs(t, err, flag.ErrHelp)
	assert.Contains(t, help.String(), "Usage: reset")

	_, err = parseArgs([]string{"a", "b"}, io.Discard)
	require.ErrorContains(t, err, "unexpected argument")

	_, err = parseArgs([]string{"-h"}, brokenWriter{})
	require.ErrorContains(t, err, "usage")
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) {
	return 0, errors.New("nowhere to write")
}
