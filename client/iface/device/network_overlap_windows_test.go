package device

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOverlappingInterfaces(t *testing.T) {
	pfx := netip.MustParsePrefix
	own := pfx("100.64.0.0/16")

	others := []interfaceNetworks{
		{name: "wt0", networks: []netip.Prefix{pfx("100.64.0.0/16")}},
		{name: "wt1", networks: []netip.Prefix{pfx("100.65.0.0/16")}},
		{name: "wt2", networks: []netip.Prefix{pfx("fd00::/64"), pfx("100.64.128.0/17")}},
		{name: "wt3", networks: nil},
	}

	assert.Equal(t, []string{"wt2"}, overlappingInterfaces(own, "wt0", others), "the interface itself is skipped")
	assert.Equal(t, []string{"wt0", "wt2"}, overlappingInterfaces(own, "wt9", others))
	assert.Empty(t, overlappingInterfaces(pfx("100.66.0.0/16"), "wt9", others))
	assert.Equal(t, []string{"wt0"}, overlappingInterfaces(own, "WT2", others[:1]), "names are compared without case")
	assert.Empty(t, overlappingInterfaces(own, "WT0", others[:1]))
}
