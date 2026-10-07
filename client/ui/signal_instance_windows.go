//go:build windows

package main

// quickActionsEventName is the show-window event of the UI for the given
// instance suffix. The default UI keeps quickActionsTriggerEventName.
func quickActionsEventName(suffix string) string {
	if suffix == "" {
		return quickActionsTriggerEventName
	}
	return quickActionsTriggerEventName + "_" + suffix
}
