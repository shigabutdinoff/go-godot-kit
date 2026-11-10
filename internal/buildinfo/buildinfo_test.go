package buildinfo

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestString(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "Build version: v1.2.3\nBuild date: 2026-01-02\nBuild commit: abc1234",
		String("v1.2.3", "2026-01-02", "abc1234"))
	assert.Equal(t, "Build version: N/A\nBuild date: N/A\nBuild commit: N/A", String("", "", ""))
}

// failWriter fails every write with errWrite.
type failWriter struct{}

var errWrite = errors.New("write failed")

func (failWriter) Write([]byte) (int, error) { return 0, errWrite }

func TestWrite(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	require.NoError(t, Write(&buf, "v1.2.3", "", ""))
	assert.Equal(t, "Build version: v1.2.3\nBuild date: N/A\nBuild commit: N/A\n", buf.String())
	assert.ErrorIs(t, Write(failWriter{}, "", "", ""), errWrite)
}
