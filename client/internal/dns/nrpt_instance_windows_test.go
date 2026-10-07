package dns

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInstanceKeyPrefix(t *testing.T) {
	tests := []struct {
		iface string
		want  string
	}{
		{"", ""},
		{"wt0", ""},
		{"WT0", ""},
		{"wt1", "NBI-wt1-"},
		{"WT1", "NBI-wt1-"},
		{"wt10", "NBI-wt10-"},
	}

	for _, tc := range tests {
		assert.Equal(t, tc.want, instanceKeyPrefix(tc.iface), "interface %q", tc.iface)
	}
}

// The default instance deletes rule keys by prefix, so no other instance's key
// may start with it, and no instance's prefix may be a prefix of another's.
func TestInstancePrefixesDoNotOverlap(t *testing.T) {
	prefixes := []string{NRPTKeyPrefix, instanceKeyPrefix("wt1"), instanceKeyPrefix("wt10"), instanceKeyPrefix("wt2")}

	for i, a := range prefixes {
		for j, b := range prefixes {
			if i == j {
				continue
			}
			assert.False(t, strings.HasPrefix(strings.ToLower(a), strings.ToLower(b)), "%q must not start with %q", a, b)
		}
	}
}

// A default instance must keep writing exactly the keys older versions did.
func TestDefaultInstancePathsUnchanged(t *testing.T) {
	r := &registryConfigurator{}

	assert.Equal(t, dnsPolicyConfigMatchPath, r.matchPath(DNSPolicyConfigRoot))
	assert.Equal(t, gpoDnsPolicyConfigMatchPath, r.matchPath(GPODNSPolicyConfigRoot))
	assert.Equal(t, dnsPolicyConfigExemptLocalPath, r.exemptLocalPath(DNSPolicyConfigRoot))
	assert.Equal(t, gpoDnsPolicyConfigExemptLocalPath, r.exemptLocalPath(GPODNSPolicyConfigRoot))
}

func TestInstancePaths(t *testing.T) {
	r := &registryConfigurator{keyPrefix: instanceKeyPrefix("wt1")}

	assert.Equal(t, DNSPolicyConfigRoot+`\NBI-wt1-Match`, r.matchPath(DNSPolicyConfigRoot))
	assert.Equal(t, GPODNSPolicyConfigRoot+`\NBI-wt1-ExemptLocal`, r.exemptLocalPath(GPODNSPolicyConfigRoot))
}
