package cmd

import (
	"fmt"
	"strings"
)

// serviceDisplayName is the name the service manager lists. The default service
// keeps "Netbird"; any other adds its service name, so several installed
// instances can be told apart.
func serviceDisplayName(name string) string {
	if strings.EqualFold(name, "netbird") {
		return "Netbird"
	}
	return fmt.Sprintf("Netbird (%s)", name)
}
