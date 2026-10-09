package cmd

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os/exec"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/netbirdio/netbird/client/internal/dns"
	"github.com/netbirdio/netbird/client/internal/wincmd"
)

func init() {
	rootCmd.AddCommand(instancesCmd)
}

var instancesCmd = &cobra.Command{
	Use:   "instances",
	Short: "List the NetBird instances on this machine and the problems found with them",
	Long: "Lists every installed NetBird service with its daemon address and state directory, the NetBird " +
		"interfaces that are up, the DNS rules of each instance and the firewall profile of each interface, " +
		"and warns about settings that would make instances interfere with each other. Read only.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		report := gatherInstanceReport()
		cmd.Print(report.String())
		return nil
	},
}

// serviceStatus is an installed NetBird service and whether it runs.
type serviceStatus struct {
	instanceInfo
	running bool
}

// adapterInfo is a NetBird interface that exists on the machine.
type adapterInfo struct {
	name  string
	up    bool
	addrs []string
}

type instanceReport struct {
	services []serviceStatus
	adapters []adapterInfo
	// profiles maps an interface alias to its network category, e.g. "Public".
	profiles map[string]string
	nrpt     []dns.NRPTInstance
}

func gatherInstanceReport() instanceReport {
	return instanceReport{
		services: listServiceStatuses(),
		adapters: listAdapters(),
		profiles: listNetworkCategories(),
		nrpt:     dns.ListNRPTInstances(),
	}
}

func listServiceStatuses() []serviceStatus {
	m, err := mgr.Connect()
	if err != nil {
		return nil
	}
	defer func() { _ = m.Disconnect() }()

	names, err := m.ListServices()
	if err != nil {
		return nil
	}

	var out []serviceStatus
	for _, name := range names {
		info, ok := readInstance(m, name)
		if !ok {
			continue
		}
		st := serviceStatus{instanceInfo: info}
		if s, err := m.OpenService(name); err == nil {
			if q, err := s.Query(); err == nil {
				st.running = q.State == svc.Running
			}
			_ = s.Close()
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

func listAdapters() []adapterInfo {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	var out []adapterInfo
	for _, iface := range ifaces {
		if !strings.HasPrefix(strings.ToLower(iface.Name), "wt") {
			continue
		}
		a := adapterInfo{name: iface.Name, up: iface.Flags&net.FlagUp != 0}
		if addrs, err := iface.Addrs(); err == nil {
			for _, addr := range addrs {
				a.addrs = append(a.addrs, addr.String())
			}
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// listNetworkCategories reads the firewall profile of every connection. A
// failure yields an empty map: the report then says the profile is unknown.
func listNetworkCategories() map[string]string {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	powershell := wincmd.System32(`WindowsPowerShell\v1.0\powershell`)
	script := `Get-NetConnectionProfile | ForEach-Object { "$($_.InterfaceAlias)=$($_.NetworkCategory)" }`
	cmd := exec.CommandContext(ctx, powershell, "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	out, err := cmd.Output()
	if err != nil {
		return map[string]string{}
	}
	return parseNetworkCategories(string(out))
}

func parseNetworkCategories(out string) map[string]string {
	profiles := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		alias, category, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && alias != "" {
			profiles[strings.ToLower(alias)] = category
		}
	}
	return profiles
}

// warnings lists the problems the report shows. Each names what is wrong and
// what to change.
func (r instanceReport) warnings() []string {
	var w []string

	for i, a := range r.services {
		for _, b := range r.services[i+1:] {
			if a.daemonAddr == b.daemonAddr {
				w = append(w, fmt.Sprintf("services %s and %s use the same daemon address %s", a.name, b.name, a.daemonAddr))
			}
			if sameDir(a.stateDir, b.stateDir) {
				w = append(w, fmt.Sprintf("services %s and %s share the state directory %s", a.name, b.name, a.stateDir))
			}
		}
	}

	for _, a := range r.adapters {
		if !a.up {
			continue
		}
		switch category, known := r.profiles[strings.ToLower(a.name)]; {
		case !known:
			w = append(w, fmt.Sprintf("the firewall profile of interface %s is unknown", a.name))
		case !strings.EqualFold(category, "Private") && !strings.EqualFold(category, "DomainAuthenticated"):
			w = append(w, fmt.Sprintf("interface %s is on the %s firewall profile: policy may block inbound traffic, "+
				"see 'Set-NetConnectionProfile -InterfaceAlias %s -NetworkCategory Private'", a.name, category, a.name))
		}
	}

	for i, a := range r.adapters {
		for _, b := range r.adapters[i+1:] {
			if a.up && b.up && networksOverlap(a.addrs, b.addrs) {
				w = append(w, fmt.Sprintf("the networks of interfaces %s and %s overlap: the two NetBird networks "+
					"cannot both be reached reliably", a.name, b.name))
			}
		}
	}

	live := make([]dns.NRPTInstance, 0, len(r.nrpt))
	for _, n := range r.nrpt {
		if n.Live {
			live = append(live, n)
		}
	}
	for i, a := range live {
		for _, b := range live[i+1:] {
			w = append(w, overlapWarnings(a, b)...)
		}
	}

	return w
}

// networksOverlap reports whether any address network of a overlaps one of b.
// Addresses that are not in CIDR form are ignored.
func networksOverlap(a, b []string) bool {
	for _, x := range a {
		px, err := netip.ParsePrefix(x)
		if err != nil {
			continue
		}
		for _, y := range b {
			if py, err := netip.ParsePrefix(y); err == nil && px.Masked().Overlaps(py.Masked()) {
				return true
			}
		}
	}
	return false
}

// overlapWarnings lists the DNS conflicts between two live instances. One
// instance may hold the catch-all, since the most specific rule wins; two cannot.
func overlapWarnings(a, b dns.NRPTInstance) []string {
	var w []string
	if a.HasCatchAll() && b.HasCatchAll() {
		w = append(w, fmt.Sprintf("instances %s and %s both resolve every domain (.): only one of them can", a.Interface, b.Interface))
	}
	for _, x := range a.Namespaces {
		for _, y := range b.Namespaces {
			if x != "." && y != "." && dns.NamespacesOverlap(x, y) {
				w = append(w, fmt.Sprintf("DNS domain %s of %s overlaps %s of %s", x, a.Interface, y, b.Interface))
			}
		}
	}
	return w
}

func (r instanceReport) String() string {
	var sb strings.Builder

	sb.WriteString("Services\n")
	if len(r.services) == 0 {
		sb.WriteString("  none found\n")
	}
	for _, s := range r.services {
		state := "stopped"
		if s.running {
			state = "running"
		}
		fmt.Fprintf(&sb, "  %s (%s)\n    daemon address: %s\n    state directory: %s\n", s.name, state, s.daemonAddr, s.stateDir)
	}

	sb.WriteString("\nInterfaces\n")
	if len(r.adapters) == 0 {
		sb.WriteString("  none found\n")
	}
	for _, a := range r.adapters {
		state := "down"
		if a.up {
			state = "up"
		}
		category := r.profiles[strings.ToLower(a.name)]
		if category == "" {
			category = "unknown"
		}
		fmt.Fprintf(&sb, "  %s (%s), firewall profile %s, addresses %s\n", a.name, state, category, strings.Join(a.addrs, ", "))
	}

	sb.WriteString("\nDNS rules\n")
	if len(r.nrpt) == 0 {
		sb.WriteString("  none found\n")
	}
	for _, n := range r.nrpt {
		note := ""
		if !n.Live {
			note = " (interface is down, leftover rules)"
		}
		fmt.Fprintf(&sb, "  %s%s: %s\n", n.Interface, note, strings.Join(n.Namespaces, ", "))
	}

	sb.WriteString("\nProblems\n")
	warnings := r.warnings()
	if len(warnings) == 0 {
		sb.WriteString("  none found\n")
	}
	for _, w := range warnings {
		fmt.Fprintf(&sb, "  - %s\n", w)
	}
	return sb.String()
}
