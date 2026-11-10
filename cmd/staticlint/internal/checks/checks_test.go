package checks

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func names(t *testing.T, data string) []string {
	t.Helper()
	as, err := Staticcheck([]byte(data))
	require.NoError(t, err)
	var ns []string
	for _, a := range as {
		ns = append(ns, a.Analyzer.Name)
	}
	return ns
}

func TestStaticcheck_Default(t *testing.T) {
	t.Parallel()
	prefixes := map[string]bool{}
	for _, n := range names(t, string(Default)) {
		assert.NotEqual(t, "ST1000", n)
		prefixes[strings.TrimRight(n, "0123456789")] = true
	}
	assert.Equal(t, map[string]bool{"SA": true, "S": true, "ST": true, "QF": true}, prefixes, "all four groups")
}

func TestStaticcheck(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"SA4006", "SA5000"}, names(t, `{"staticcheck": ["SA5000", "SA4006"]}`),
		"listed checks in staticcheck order")
	assert.Empty(t, names(t, `{"staticcheck": ["SA5000", "-SA5000"]}`), "a later -NAME disables")
	assert.Empty(t, names(t, `{"staticcheck": []}`))
	for _, bad := range []string{`{"staticcheck": ["SA9999"]}`, `{"checks": []}`, `not json`} {
		_, err := Staticcheck([]byte(bad))
		assert.Error(t, err, bad)
	}
}
