package libgodot

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestName(t *testing.T) {
	t.Parallel()
	for goos, want := range map[string]string{
		"darwin": "libgodot.dylib", "linux": "libgodot.so", "windows": "libgodot.dll", "js": "",
	} {
		assert.Equal(t, want, Name(goos), goos)
	}
}
