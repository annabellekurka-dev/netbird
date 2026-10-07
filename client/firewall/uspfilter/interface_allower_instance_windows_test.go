package uspfilter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFirewallRuleNameFor(t *testing.T) {
	tests := []struct {
		iface string
		want  string
	}{
		{"", "Netbird"},
		{"wt0", "Netbird"},
		{"WT0", "Netbird"},
		{"wt1", "Netbird-wt1"},
		{"WT1", "Netbird-wt1"},
	}

	for _, tc := range tests {
		assert.Equal(t, tc.want, firewallRuleNameFor(tc.iface), "interface %q", tc.iface)
	}
}

// The v6 rule is the instance's rule plus "-v6", so no instance's v4 name may
// equal another's v6 name.
func TestFirewallRuleNamesDoNotCollide(t *testing.T) {
	seen := map[string]string{}
	for _, iface := range []string{"wt0", "wt1", "wt2", "wt10"} {
		for _, name := range []string{firewallRuleNameFor(iface), firewallRuleNameFor(iface) + "-v6"} {
			other, dup := seen[name]
			assert.False(t, dup, "%q used by both %q and %q", name, other, iface)
			seen[name] = iface
		}
	}
}
