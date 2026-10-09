package dns

import (
	"fmt"
	"net"
	"sort"
	"strings"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/windows/registry"

	"github.com/netbirdio/netbird/client/iface/configurer"
)

// foreignInstance is another instance that has NRPT rules installed.
type foreignInstance struct {
	iface      string
	namespaces []string
}

// dnsConflicts compares the namespaces this instance is about to claim with
// those of the other instances. Windows picks the most specific rule, so one
// instance may hold the catch-all while the others resolve their own domains.
// Two catch-alls cannot both win, so that is an error; sharing a namespace, or
// nesting one in another, is only worth a warning.
func dnsConflicts(own []string, routeAll bool, others []foreignInstance) (warnings []string, err error) {
	if routeAll {
		for _, other := range others {
			if hasCatchAll(other.namespaces) {
				return nil, fmt.Errorf("a nameserver group for every domain (\".\") cannot be applied while another NetBird "+
					"instance (%s) already holds the catch-all: only one instance can resolve every other name", other.iface)
			}
		}
	}

	for _, other := range others {
		for _, ns := range own {
			for _, theirs := range other.namespaces {
				if overlaps(ns, theirs) {
					warnings = append(warnings, fmt.Sprintf("DNS domain %s overlaps %s of instance %s", ns, theirs, other.iface))
				}
			}
		}
	}
	return warnings, nil
}

// hasCatchAll reports whether the namespaces include the catch-all.
func hasCatchAll(namespaces []string) bool {
	for _, ns := range namespaces {
		if ns == nrptCatchAllNamespace {
			return true
		}
	}
	return false
}

// HasCatchAll reports whether the instance resolves every name that no more
// specific rule claims.
func (n NRPTInstance) HasCatchAll() bool {
	return hasCatchAll(n.Namespaces)
}

// overlaps reports whether two namespaces are equal or one is inside the other.
func overlaps(a, b string) bool {
	a, b = strings.ToLower(strings.TrimSuffix(a, ".")), strings.ToLower(strings.TrimSuffix(b, "."))
	if a == "" || b == "" || a == nrptLocalNamespace || b == nrptLocalNamespace {
		return false
	}
	a, b = "."+strings.TrimPrefix(a, "."), "."+strings.TrimPrefix(b, ".")
	return strings.HasSuffix(a, b) || strings.HasSuffix(b, a)
}

// checkInstanceConflicts logs the overlaps between this instance's DNS config
// and that of the instances running next to it, and refuses a catch-all.
func (r *registryConfigurator) checkInstanceConflicts(config HostDNSConfig) error {
	var own []string
	for _, d := range config.Domains {
		if !d.Disabled {
			own = append(own, "."+strings.TrimSuffix(d.Domain, "."))
		}
	}

	others := r.foreignInstances()
	if len(others) == 0 {
		return nil
	}

	warnings, err := dnsConflicts(own, config.RouteAll, others)
	for _, w := range warnings {
		log.Warn(w)
	}
	return err
}

// NRPTInstance is a NetBird instance that has NRPT rules installed.
type NRPTInstance struct {
	Interface  string
	Namespaces []string
	// Live is false when the instance's interface is down, which means the rules
	// are leftovers of a crashed instance.
	Live bool
}

// NamespacesOverlap reports whether two NRPT namespaces are equal or nested.
func NamespacesOverlap(a, b string) bool {
	return overlaps(a, b)
}

// ListNRPTInstances reads the rules of every NetBird instance from the registry,
// the only place they are all recorded, ordered by interface name.
func ListNRPTInstances() []NRPTInstance {
	byIface := map[string]*NRPTInstance{}

	for _, root := range []string{DNSPolicyConfigRoot, GPODNSPolicyConfigRoot} {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, root, registry.ENUMERATE_SUB_KEYS)
		if err != nil {
			continue
		}
		names, err := k.ReadSubKeyNames(-1)
		_ = k.Close()
		if err != nil {
			continue
		}

		for _, name := range names {
			iface, ok := ruleOwnerInterface(name)
			if !ok {
				continue
			}

			rule, err := registry.OpenKey(registry.LOCAL_MACHINE, root+`\`+name, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			namespaces, _, err := rule.GetStringsValue(dnsPolicyConfigNameKey)
			_ = rule.Close()
			if err != nil {
				continue
			}

			inst := byIface[iface]
			if inst == nil {
				inst = &NRPTInstance{Interface: iface, Live: interfaceIsUp(iface)}
				byIface[iface] = inst
			}
			inst.Namespaces = append(inst.Namespaces, namespaces...)
		}
	}

	instances := make([]NRPTInstance, 0, len(byIface))
	for _, inst := range byIface {
		instances = append(instances, *inst)
	}
	sort.Slice(instances, func(i, j int) bool { return instances[i].Interface < instances[j].Interface })
	return instances
}

// foreignInstances returns the other instances whose rules count. A rule counts
// only while the interface of its owner is up, so keys a crashed instance left
// behind are ignored.
func (r *registryConfigurator) foreignInstances() []foreignInstance {
	own, _ := ruleOwnerInterface(r.nrptPrefix() + "Match")

	var others []foreignInstance
	for _, inst := range ListNRPTInstances() {
		if !inst.Live || strings.EqualFold(inst.Interface, own) {
			continue
		}
		others = append(others, foreignInstance{iface: inst.Interface, namespaces: inst.Namespaces})
	}
	return others
}

// ruleOwnerInterface returns the interface of the instance that wrote the rule
// key, or false for a key that is not one of ours or only exempts a namespace.
func ruleOwnerInterface(keyName string) (string, bool) {
	lower := strings.ToLower(keyName)
	if strings.HasSuffix(lower, "exemptlocal") {
		return "", false
	}

	if strings.HasPrefix(lower, strings.ToLower(NRPTKeyPrefix)) {
		return configurer.WgInterfaceDefault, true
	}

	if rest, ok := strings.CutPrefix(lower, strings.ToLower(NRPTInstanceKeyPrefix)); ok {
		if iface, _, found := strings.Cut(rest, "-"); found && iface != "" {
			return iface, true
		}
	}
	return "", false
}

func interfaceIsUp(name string) bool {
	iface, err := net.InterfaceByName(name)
	return err == nil && iface.Flags&net.FlagUp != 0
}
