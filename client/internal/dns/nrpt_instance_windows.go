package dns

import (
	"strings"

	"github.com/netbirdio/netbird/client/iface/configurer"
)

// NRPTInstanceKeyPrefix starts the NRPT rule key names of every instance that
// does not run on the default interface. It must not start with NRPTKeyPrefix:
// the default instance deletes its rule keys by that prefix and would take
// these with them.
const NRPTInstanceKeyPrefix = "NBI-"

// instanceKeyPrefix returns the rule key prefix for the instance on the named
// interface, or "" for the default interface, which keeps NRPTKeyPrefix. The
// trailing dash keeps one instance's prefix from matching another's keys, as
// "wt1" would otherwise match "wt10".
func instanceKeyPrefix(ifaceName string) string {
	if ifaceName == "" || strings.EqualFold(ifaceName, configurer.WgInterfaceDefault) {
		return ""
	}
	return NRPTInstanceKeyPrefix + strings.ToLower(ifaceName) + "-"
}
