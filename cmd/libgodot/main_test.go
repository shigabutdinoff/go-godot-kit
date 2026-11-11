package main

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_Errors(t *testing.T) {
	t.Parallel()
	noModule := func(context.Context) (string, error) { return "", errors.New("run inside a Go module") }
	for _, tt := range []struct {
		name    string
		args    []string
		wantErr string // empty means no error
		wantOut string
	}{
		{name: "unknown flag", args: []string{"-bad"}, wantErr: "-bad"},
		{name: "extra argument", args: []string{"extra"}, wantErr: "extra"},
		{name: "help", args: []string{"-h"}, wantOut: "-force"},
		{name: "outside a module", wantErr: "run inside a Go module"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			err := run(t.Context(), tt.args, &out, noModule)
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantErr)
			}
			assert.Contains(t, out.String(), tt.wantOut)
		})
	}
}
