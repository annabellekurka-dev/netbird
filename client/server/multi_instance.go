package server

import (
	"strings"

	"github.com/netbirdio/netbird/client/iface/configurer"
)

// hasOtherInterface reports whether any interface other than own looks like the
// interface of another NetBird instance. NetBird interface names on Windows
// all start with the default's two-letter prefix, "wt".
func hasOtherInterface(names []string, own string) bool {
	prefix := strings.ToLower(configurer.WgInterfaceDefault[:2])
	for _, name := range names {
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, prefix) && !strings.EqualFold(name, own) {
			return true
		}
	}
	return false
}
