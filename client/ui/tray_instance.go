//go:build !android && !ios && !freebsd && !js

package main

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"
)

// trayPollInterval is how often the tray re-reads state that changes when another
// NetBird instance starts or stops: the daemon only tells the tray about changes
// to its own configuration, so a second instance coming up would go unnoticed.
const trayPollInterval = 15 * time.Second

var (
	activeProfileMu   sync.Mutex
	activeProfileName string
)

// noteActiveProfile records the profile the daemon is on and shows it in the tooltip.
func (t *Tray) noteActiveProfile(name string) {
	activeProfileMu.Lock()
	changed := activeProfileName != name
	activeProfileName = name
	activeProfileMu.Unlock()

	if changed {
		t.updateTooltip()
	}
}

// updateTooltip sets the text shown when hovering the tray icon.
func (t *Tray) updateTooltip() {
	activeProfileMu.Lock()
	profile := activeProfileName
	activeProfileMu.Unlock()

	label := instanceTooltipLabel(flagInstanceSuffix(), profile, netbirdInterfacesUp())
	t.tray.SetTooltip(withInstanceLabel(t.loc.T("tray.tooltip"), label))
}

// pollInstanceState keeps the parts of the tray that depend on the other NetBird
// instances current.
func (t *Tray) pollInstanceState(ctx context.Context) {
	ticker := time.NewTicker(trayPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.refreshRestrictions()
			t.updateTooltip()
		}
	}
}

// instanceTooltipLabel says which network a tray belongs to. A single default
// instance gets no label, so its tooltip stays what it always was. Once there is
// more than one NetBird interface, or this tray is for another daemon, the
// profile name tells the trays apart, and the instance suffix does if the
// profile is not known yet.
func instanceTooltipLabel(suffix, profile string, interfacesUp int) string {
	if suffix == "" && interfacesUp < 2 {
		return ""
	}
	if profile != "" {
		return profile
	}
	return suffix
}

// netbirdInterfacesUp counts the NetBird interfaces ("wt*") that are up.
func netbirdInterfacesUp() int {
	ifaces, err := net.Interfaces()
	if err != nil {
		return 0
	}

	count := 0
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp != 0 && strings.HasPrefix(strings.ToLower(iface.Name), "wt") {
			count++
		}
	}
	return count
}
