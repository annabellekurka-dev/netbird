package dns

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDNSConflicts(t *testing.T) {
	other := []foreignInstance{{iface: "wt0", namespaces: []string{".work.bird", ".corp.example.com"}}}

	t.Run("no other instance", func(t *testing.T) {
		warnings, err := dnsConflicts([]string{".priv.bird"}, false, nil)
		require.NoError(t, err)
		assert.Empty(t, warnings)
	})

	t.Run("distinct domains", func(t *testing.T) {
		warnings, err := dnsConflicts([]string{".priv.bird"}, false, other)
		require.NoError(t, err)
		assert.Empty(t, warnings)
	})

	t.Run("look-alike suffix is not an overlap", func(t *testing.T) {
		warnings, err := dnsConflicts([]string{".evilwork.bird"}, false, other)
		require.NoError(t, err)
		assert.Empty(t, warnings)
	})

	t.Run("same domain", func(t *testing.T) {
		warnings, err := dnsConflicts([]string{".WORK.bird."}, false, other)
		require.NoError(t, err)
		assert.Len(t, warnings, 1)
	})

	t.Run("nested domain", func(t *testing.T) {
		warnings, err := dnsConflicts([]string{".a.corp.example.com"}, false, other)
		require.NoError(t, err)
		assert.Len(t, warnings, 1)
	})

	t.Run("a catch-all next to specific domains is fine", func(t *testing.T) {
		warnings, err := dnsConflicts([]string{".priv.bird"}, true, other)
		require.NoError(t, err)
		assert.Empty(t, warnings)
	})

	t.Run("two catch-alls are refused", func(t *testing.T) {
		withCatchAll := []foreignInstance{{iface: "wt0", namespaces: []string{".work.bird", "."}}}
		_, err := dnsConflicts([]string{".priv.bird"}, true, withCatchAll)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "wt0")
	})

	t.Run("the other instance's catch-all does not block specific domains", func(t *testing.T) {
		withCatchAll := []foreignInstance{{iface: "wt0", namespaces: []string{".work.bird", "."}}}
		warnings, err := dnsConflicts([]string{".priv.bird"}, false, withCatchAll)
		require.NoError(t, err)
		assert.Empty(t, warnings)
	})

	t.Run("catch-all alone is fine", func(t *testing.T) {
		_, err := dnsConflicts(nil, true, nil)
		assert.NoError(t, err)
	})
}

func TestRuleOwnerInterface(t *testing.T) {
	tests := []struct {
		key   string
		iface string
		ok    bool
	}{
		{"NetBird-Match-0", "wt0", true},
		{"netbird-match-1", "wt0", true},
		{"NBI-wt1-Match-0", "wt1", true},
		{"NBI-wt10-Match-0", "wt10", true},
		{"NetBird-ExemptLocal", "", false},
		{"NBI-wt1-ExemptLocal", "", false},
		{"SomeOtherProduct-Rule", "", false},
		{"NBI-", "", false},
	}

	for _, tc := range tests {
		iface, ok := ruleOwnerInterface(tc.key)
		assert.Equal(t, tc.ok, ok, tc.key)
		assert.Equal(t, tc.iface, iface, tc.key)
	}
}
