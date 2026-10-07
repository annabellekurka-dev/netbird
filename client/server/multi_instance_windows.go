package server

import (
	"net"

	log "github.com/sirupsen/logrus"
)

// otherInstanceActive reports whether another NetBird connection is up on this
// machine. An adapter lookup failure counts as "no", so a transient error never
// hides a feature.
func otherInstanceActive(own string) bool {
	ifaces, err := net.Interfaces()
	if err != nil {
		log.Debugf("list interfaces to detect other NetBird instances: %v", err)
		return false
	}

	names := make([]string, 0, len(ifaces))
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp != 0 {
			names = append(names, iface.Name)
		}
	}
	return hasOtherInterface(names, own)
}
