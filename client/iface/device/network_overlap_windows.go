package device

import (
	"net"
	"net/netip"
	"strings"

	log "github.com/sirupsen/logrus"
)

// interfaceNetworks is a NetBird interface and the networks it is addressed in.
type interfaceNetworks struct {
	name     string
	networks []netip.Prefix
}

// overlappingInterfaces returns the names of the interfaces, other than the one
// named own, that have a network overlapping the given one. Two instances whose
// overlay networks overlap cannot both route to it.
func overlappingInterfaces(own netip.Prefix, ownName string, others []interfaceNetworks) []string {
	var names []string
	for _, other := range others {
		if strings.EqualFold(other.name, ownName) {
			continue
		}
		for _, n := range other.networks {
			if own.Overlaps(n) {
				names = append(names, other.name)
				break
			}
		}
	}
	return names
}

// warnOverlappingNetworks logs when the network of this interface overlaps that
// of another NetBird interface that is up. NetBird picks each account's network
// out of 100.64.0.0/10, so an overlap between two accounts is unlikely but
// possible, and then neither instance reaches the other's peers reliably.
func warnOverlappingNetworks(ifaceName string, own netip.Prefix) {
	ifaces, err := net.Interfaces()
	if err != nil {
		log.Debugf("list interfaces to check for overlapping networks: %v", err)
		return
	}

	var others []interfaceNetworks
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || !strings.HasPrefix(strings.ToLower(iface.Name), "wt") {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		other := interfaceNetworks{name: iface.Name}
		for _, addr := range addrs {
			if p, err := netip.ParsePrefix(addr.String()); err == nil {
				other.networks = append(other.networks, p.Masked())
			}
		}
		others = append(others, other)
	}

	for _, name := range overlappingInterfaces(own.Masked(), ifaceName, others) {
		log.Warnf("the network %s of interface %s overlaps a network of interface %s: the two NetBird networks "+
			"cannot both be reached reliably, give one account another range", own.Masked(), ifaceName, name)
	}
}
