//go:build !android && !ios && !freebsd && !js

package main

import (
	"flag"
	"strings"
)

const defaultUniqueID = "io.netbird.ui"

// instanceSuffix names the daemon this UI talks to, or "" for the default one.
// Several UIs run side by side, one per daemon, so whatever must be unique per
// process (the single-instance lock, the show-window event) and whatever tells
// the user which network a tray belongs to is derived from the daemon address.
func instanceSuffix(addr string) string {
	if addr == "" || addr == DaemonAddr() {
		return ""
	}

	if _, rest, ok := strings.Cut(addr, "://"); ok {
		addr = rest
	}

	var sb strings.Builder
	for _, r := range strings.ToLower(addr) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			sb.WriteRune(r)
		default:
			sb.WriteByte('-')
		}
	}

	suffix := strings.Trim(sb.String(), "-")
	if len(suffix) > 40 {
		suffix = suffix[:40]
	}
	return suffix
}

// flagInstanceSuffix is the suffix of the --daemon-addr this process was started with.
func flagInstanceSuffix() string {
	f := flag.Lookup("daemon-addr")
	if f == nil {
		return ""
	}
	return instanceSuffix(f.Value.String())
}

// uniqueID is the single-instance lock of this process. The default UI keeps its
// ID, so a second launch of it still opens the first one's window.
func uniqueID(suffix string) string {
	if suffix == "" {
		return defaultUniqueID
	}
	return defaultUniqueID + "." + suffix
}

// withInstanceLabel appends the instance to a tooltip or title, so the trays of
// two networks can be told apart.
func withInstanceLabel(text, suffix string) string {
	if suffix == "" {
		return text
	}
	return text + " (" + suffix + ")"
}
