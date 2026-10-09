package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netbirdio/netbird/client/internal/dns"
)

func TestParseNetworkCategories(t *testing.T) {
	got := parseNetworkCategories("Ethernet=Private\r\nwt1=Public\r\n\r\nnoequals\r\n=x\r\n")

	assert.Equal(t, map[string]string{"ethernet": "Private", "wt1": "Public"}, got)
}

func TestInstanceReportWarnings(t *testing.T) {
	healthy := instanceReport{
		services: []serviceStatus{
			{instanceInfo: instanceInfo{name: "Netbird", daemonAddr: "npipe://netbird", stateDir: `C:\ProgramData\Netbird`}},
			{instanceInfo: instanceInfo{name: "NetBird-private", daemonAddr: "npipe://netbird-private", stateDir: `C:\ProgramData\Netbird-private`}},
		},
		adapters: []adapterInfo{{name: "wt0", up: true}, {name: "wt1", up: true}},
		profiles: map[string]string{"wt0": "Private", "wt1": "Private"},
		nrpt: []dns.NRPTInstance{
			{Interface: "wt0", Namespaces: []string{".work.bird"}, Live: true},
			{Interface: "wt1", Namespaces: []string{".priv.bird"}, Live: true},
		},
	}
	assert.Empty(t, healthy.warnings())

	t.Run("shared address and state directory", func(t *testing.T) {
		r := healthy
		r.services = []serviceStatus{
			{instanceInfo: instanceInfo{name: "A", daemonAddr: "npipe://x", stateDir: `C:\s`}},
			{instanceInfo: instanceInfo{name: "B", daemonAddr: "npipe://x", stateDir: `c:\S\`}},
		}
		assert.Len(t, r.warnings(), 2)
	})

	t.Run("public firewall profile", func(t *testing.T) {
		r := healthy
		r.profiles = map[string]string{"wt0": "Private", "wt1": "Public"}
		w := r.warnings()
		assert.Len(t, w, 1)
		assert.Contains(t, w[0], "wt1")
	})

	t.Run("unknown profile of an up interface", func(t *testing.T) {
		r := healthy
		r.profiles = map[string]string{}
		assert.Len(t, r.warnings(), 2)
	})

	t.Run("a down interface is not checked", func(t *testing.T) {
		r := healthy
		r.adapters = []adapterInfo{{name: "wt1", up: false}}
		r.profiles = map[string]string{}
		assert.Empty(t, r.warnings())
	})

	t.Run("overlapping networks", func(t *testing.T) {
		r := healthy
		r.adapters = []adapterInfo{
			{name: "wt0", up: true, addrs: []string{"100.64.1.5/16", "fe80::1/64"}},
			{name: "wt1", up: true, addrs: []string{"100.64.200.9/16"}},
		}
		w := r.warnings()
		assert.Len(t, w, 1)
		assert.Contains(t, w[0], "networks of interfaces wt0 and wt1 overlap")
	})

	t.Run("distinct networks", func(t *testing.T) {
		r := healthy
		r.adapters = []adapterInfo{
			{name: "wt0", up: true, addrs: []string{"100.64.1.5/16"}},
			{name: "wt1", up: true, addrs: []string{"100.65.1.5/16"}},
		}
		assert.Empty(t, r.warnings())
	})

	t.Run("overlapping domains", func(t *testing.T) {
		r := healthy
		r.nrpt = []dns.NRPTInstance{
			{Interface: "wt0", Namespaces: []string{".work.bird"}, Live: true},
			{Interface: "wt1", Namespaces: []string{".sub.work.bird"}, Live: true},
		}
		assert.Len(t, r.warnings(), 1)
	})

	t.Run("one catch-all next to specific domains is fine", func(t *testing.T) {
		r := healthy
		r.nrpt = []dns.NRPTInstance{
			{Interface: "wt0", Namespaces: []string{".work.bird", "."}, Live: true},
			{Interface: "wt1", Namespaces: []string{".priv.bird"}, Live: true},
		}
		assert.Empty(t, r.warnings())
	})

	t.Run("two catch-alls", func(t *testing.T) {
		r := healthy
		r.nrpt = []dns.NRPTInstance{
			{Interface: "wt0", Namespaces: []string{".work.bird", "."}, Live: true},
			{Interface: "wt1", Namespaces: []string{".priv.bird", "."}, Live: true},
		}
		w := r.warnings()
		assert.Len(t, w, 1)
		assert.Contains(t, w[0], "both resolve every domain")
	})

	t.Run("leftover rules of a dead instance are ignored", func(t *testing.T) {
		r := healthy
		r.nrpt = []dns.NRPTInstance{
			{Interface: "wt0", Namespaces: []string{".work.bird"}, Live: true},
			{Interface: "wt1", Namespaces: []string{".work.bird"}, Live: false},
		}
		assert.Empty(t, r.warnings())
	})
}

func TestInstanceReportString(t *testing.T) {
	out := instanceReport{}.String()

	for _, section := range []string{"Services", "Interfaces", "DNS rules", "Problems"} {
		assert.Contains(t, out, section)
	}
}
