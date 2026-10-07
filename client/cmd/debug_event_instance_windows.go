package cmd

import (
	"strings"

	"github.com/netbirdio/netbird/client/iface/configurer"
)

// debugTriggerEventNameFor returns the debug trigger event name of the instance
// on the named interface. The default interface keeps debugTriggerEventName;
// any other gets its own, so each instance listens on an event of its own.
func debugTriggerEventNameFor(ifaceName string) string {
	if ifaceName == "" || strings.EqualFold(ifaceName, configurer.WgInterfaceDefault) {
		return debugTriggerEventName
	}
	return debugTriggerEventName + "_" + strings.ToLower(ifaceName)
}
