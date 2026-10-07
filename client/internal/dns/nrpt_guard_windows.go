package dns

import (
	"fmt"
	"net"
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
// those of the other instances. Taking the catch-all while another instance
// has rules would send every name the other instance resolves through this
// one, so it is an error; sharing a namespace, or nesting one in another, is
// only worth a warning, since the more specific rule still wins.
func dnsConflicts(own []string, routeAll bool, others []foreignInstance) (warnings []string, err error) {
	if routeAll && len(others) > 0 {
		return nil, fmt.Errorf("a nameserver group for every domain (\".\") cannot be applied while another NetBird "+
			"instance (%s) resolves its own domains: it would send all of that instance's names through this one", others[0].iface)
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

// foreignInstances reads the rules of the other instances from the registry,
// the only place they are all recorded. A rule counts only while the interface
// of its owner is up, so keys a crashed instance left behind are ignored.
func (r *registryConfigurator) foreignInstances() []foreignInstance {
	byIface := map[string]*foreignInstance{}

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
			if !ok || strings.HasPrefix(strings.ToLower(name), strings.ToLower(r.nrptPrefix())) || !interfaceIsUp(iface) {
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
				inst = &foreignInstance{iface: iface}
				byIface[iface] = inst
			}
			inst.namespaces = append(inst.namespaces, namespaces...)
		}
	}

	others := make([]foreignInstance, 0, len(byIface))
	for _, inst := range byIface {
		others = append(others, *inst)
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
