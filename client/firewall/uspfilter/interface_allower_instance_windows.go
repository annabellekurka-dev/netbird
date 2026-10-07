package uspfilter

import (
	"strings"

	"github.com/netbirdio/netbird/client/iface/configurer"
)

// firewallRuleNameFor returns the firewall rule name of the instance on the
// named interface. The default interface keeps firewallRuleName; any other gets
// its own, so one instance neither finds the rule of another already active nor
// deletes it when it closes.
func firewallRuleNameFor(ifaceName string) string {
	if ifaceName == "" || strings.EqualFold(ifaceName, configurer.WgInterfaceDefault) {
		return firewallRuleName
	}
	return firewallRuleName + "-" + strings.ToLower(ifaceName)
}
