package errwriter

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failWriter fails every write with its error.
type failWriter struct{ err error }

// Write returns 0 and f.err.
func (f failWriter) Write([]byte) (int, error) { return 0, f.err }

func TestWriter(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	ok := New(&buf)
	ok.Record(fmt.Fprint(ok, "hi"))
	require.NoError(t, ok.Err())
	assert.Equal(t, "hi", buf.String())

	first, second := errors.New("first"), errors.New("second")
	w := New(failWriter{first})
	_, err := w.Write([]byte("x"))
	require.ErrorIs(t, err, first)
	w.Record(0, second)
	w.Record(0, nil)
	assert.ErrorIs(t, w.Err(), first, "the first error is kept")
}
