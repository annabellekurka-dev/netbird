package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sys/windows"

	"github.com/netbirdio/netbird/client/internal/profilemanager"
	"github.com/netbirdio/netbird/client/internal/wincmd"
)

const (
	daemonStartTimeout  = 40 * time.Second
	daemonStartInterval = 2 * time.Second
)

var (
	addOpts        addOptions
	addDryRun      bool
	removeKeepData bool
	removeYes      bool
	removeDryRun   bool
)

var instanceCmd = &cobra.Command{
	Use:   "instance",
	Short: "Add or remove an extra NetBird network on this machine (Windows)",
	Long: "Runs a second (third, ...) NetBird next to the default one, each with its own service, " +
		"adapter, DNS rules and tray. Needs an administrator PowerShell.",
}

var instanceAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add an extra network",
	Long: `Adds an extra network called <name>: its own service (NetBird-<name>), daemon address, folder
(C:\ProgramData\Netbird-<name>), log file, interface (next free wt1, wt2, ...) and WireGuard port, then connects it.

Sign in with --setup-key, or start from an existing profile with --from-profile (a file, or "default" for the
profile of the default instance), or give neither to sign in through the browser. Use --dry-run to see the steps
without changing anything.`,
	Example: `  netbird instance add private --setup-key <key> --management-url https://mgmt.example.com
  netbird instance add work --from-profile default
  netbird instance add work --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: runInstanceAdd,
}

var instanceRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove an extra network added with 'instance add'",
	Long: `Disconnects the network, removes its service and tray shortcut, and deletes its folder
(keys, profile and logs) unless --keep-data is given. The default instance is never touched.`,
	Args: cobra.ExactArgs(1),
	RunE: runInstanceRemove,
}

func init() {
	f := instanceAddCmd.Flags()
	f.StringVar(&addOpts.SetupKey, "setup-key", "", "setup key to register the new peer with")
	f.StringVar(&addOpts.ManagementURL, "management-url", "", "management server URL (default: the profile's, or NetBird cloud)")
	f.StringVar(&addOpts.FromProfile, "from-profile", "", `profile file to start from, or "default" for the default instance's profile`)
	f.StringVar(&addOpts.Interface, "interface", "", "interface name (default: the next free wt1, wt2, ...)")
	f.IntVar(&addOpts.Port, "wireguard-port", 0, "WireGuard port (default: the next free one after 51820)")
	f.BoolVar(&addOpts.NoShortcut, "no-shortcut", false, "do not add the tray to the startup folder")
	f.BoolVar(&addDryRun, "dry-run", false, "show the steps without changing anything")

	r := instanceRemoveCmd.Flags()
	r.BoolVar(&removeKeepData, "keep-data", false, "keep the folder with the keys, profile and logs")
	r.BoolVar(&removeYes, "yes", false, "do not ask for confirmation")
	r.BoolVar(&removeDryRun, "dry-run", false, "show the steps without changing anything")

	instanceCmd.AddCommand(instanceAddCmd, instanceRemoveCmd)
	rootCmd.AddCommand(instanceCmd)
}

func runInstanceAdd(cmd *cobra.Command, args []string) error {
	addOpts.Name = args[0]
	out := cmd.OutOrStdout()

	programData := os.Getenv("PROGRAMDATA")
	if programData == "" {
		return errors.New("PROGRAMDATA is not set")
	}
	if !addDryRun {
		if err := requireElevated(); err != nil {
			return err
		}
	}

	plan, err := planInstance(addOpts, programData, collectTaken())
	if err != nil {
		return err
	}

	source, err := resolveProfileSource(addOpts.FromProfile, programData)
	if err != nil {
		return err
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find this program: %w", err)
	}

	steps := addSteps(plan, addOpts, source, shortcutFor(exe))
	printPlan(out, fmt.Sprintf("Adding the network %q", plan.Name), plan, steps)
	if addDryRun {
		fmt.Fprintln(out, "\nDry run: nothing was changed.")
		return nil
	}

	if err := runSteps(out, exe, steps); err != nil {
		fmt.Fprintf(out, "\nThe network %q may be half set up. Remove what was done with:\n  netbird instance remove %s --yes\n", plan.Name, plan.Name)
		return err
	}

	fmt.Fprintf(out, "\nDone. Network %q is up on %s.\n", plan.Name, plan.Interface)
	fmt.Fprintf(out, "Start its tray now with:\n  \"%s\" --daemon-addr %s\n", uiExePath(exe), plan.daemonAddr())
	fmt.Fprintf(out, "Check everything with:\n  netbird instances\n")
	fmt.Fprintf(out, "Commands for this network need:  --daemon-addr %s  and  $env:NB_STATE_DIR = '%s'\n", plan.daemonAddr(), plan.StateDir)
	return nil
}

func runInstanceRemove(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	programData := os.Getenv("PROGRAMDATA")
	if programData == "" {
		return errors.New("PROGRAMDATA is not set")
	}
	if !removeDryRun {
		if err := requireElevated(); err != nil {
			return err
		}
	}

	plan, err := deriveInstance(args[0], programData)
	if err != nil {
		return err
	}
	if err := confirmInstanceExists(plan); err != nil {
		return err
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find this program: %w", err)
	}

	steps := removeSteps(plan, removeKeepData, shortcutFor(exe))
	printPlan(out, fmt.Sprintf("Removing the network %q", plan.Name), plan, steps)
	if removeDryRun {
		fmt.Fprintln(out, "\nDry run: nothing was changed.")
		return nil
	}

	if !removeYes && !askYes(cmd.InOrStdin(), out, "\nContinue? Type yes: ") {
		return errors.New("cancelled")
	}

	if err := runSteps(out, exe, steps); err != nil {
		return err
	}
	fmt.Fprintf(out, "\nDone. If its tray icon is still showing, quit it from the tray menu.\n")
	return nil
}

func printPlan(out io.Writer, title string, p instancePlan, steps []planStep) {
	fmt.Fprintf(out, "%s\n", title)
	fmt.Fprintf(out, "  service    %s\n  address    %s\n  folder     %s\n", p.Service, p.daemonAddr(), p.StateDir)
	if p.Interface != "" {
		fmt.Fprintf(out, "  interface  %s   port %d\n", p.Interface, p.Port)
	}
	fmt.Fprintln(out, "\nSteps:")
	for i, s := range steps {
		fmt.Fprintf(out, "  %d. %s\n", i+1, s.display())
	}
}

func askYes(in io.Reader, out io.Writer, prompt string) bool {
	fmt.Fprint(out, prompt)
	line, _ := bufio.NewReader(in).ReadString('\n')
	return strings.EqualFold(strings.TrimSpace(line), "yes")
}

func requireElevated() error {
	if !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("this needs an administrator PowerShell: installing a service is not possible without it")
	}
	return nil
}

// shortcutFor says where the tray shortcut goes and which program it starts.
func shortcutFor(exe string) shortcutEnv {
	sc := shortcutEnv{}
	if appData := os.Getenv("APPDATA"); appData != "" {
		sc.StartupDir = filepath.Join(appData, `Microsoft\Windows\Start Menu\Programs\Startup`)
	}
	if ui := uiExePath(exe); fileExists(ui) {
		sc.UIExe = ui
	}
	return sc
}

func uiExePath(exe string) string { return filepath.Join(filepath.Dir(exe), "netbird-ui.exe") }

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// resolveProfileSource turns --from-profile into a file that exists, or "".
func resolveProfileSource(value, programData string) (string, error) {
	switch value {
	case "":
		return "", nil
	case "default":
		value = filepath.Join(programData, "Netbird", "default.json")
	}

	if !fileExists(value) {
		return "", fmt.Errorf("profile file %s does not exist", value)
	}
	data, err := os.ReadFile(value)
	if err != nil {
		return "", fmt.Errorf("read profile %s: %w", value, err)
	}
	if !json.Valid(data) {
		return "", fmt.Errorf("%s is not a profile file (not valid JSON)", value)
	}
	return value, nil
}

// collectTaken gathers what the instances on this machine already use.
func collectTaken() takenResources {
	t := takenResources{
		Services:   map[string]bool{},
		Pipes:      map[string]bool{},
		Dirs:       map[string]bool{},
		Interfaces: map[string]bool{"wt0": true},
		Ports:      map[int]bool{firstInstancePort: true},
	}

	for _, in := range installedInstances() {
		t.Services[strings.ToLower(in.name)] = true
		if pipe, ok := strings.CutPrefix(in.daemonAddr, "npipe://"); ok {
			t.Pipes[strings.ToLower(pipe)] = true
		}
		t.Dirs[strings.ToLower(filepath.Clean(in.stateDir))] = true

		if iface, port, ok := readProfileSettings(in.stateDir); ok {
			t.Interfaces[strings.ToLower(iface)] = true
			t.Ports[port] = true
		}
	}

	if ifaces, err := net.Interfaces(); err == nil {
		for _, iface := range ifaces {
			if strings.HasPrefix(strings.ToLower(iface.Name), "wt") {
				t.Interfaces[strings.ToLower(iface.Name)] = true
			}
		}
	}
	return t
}

// readProfileSettings reads the interface and port an instance is set to use, so
// a stopped instance keeps its name and port to itself.
func readProfileSettings(stateDir string) (iface string, port int, ok bool) {
	data, err := os.ReadFile(filepath.Join(stateDir, "default.json"))
	if err != nil {
		return "", 0, false
	}

	var cfg struct {
		WgIface string
		WgPort  int
	}
	if err := json.Unmarshal(data, &cfg); err != nil || cfg.WgIface == "" {
		return "", 0, false
	}
	return cfg.WgIface, cfg.WgPort, true
}

// confirmInstanceExists makes sure "instance remove" only touches an instance
// that "instance add" would have created: right service, address and folder.
func confirmInstanceExists(p instancePlan) error {
	var names []string
	for _, in := range installedInstances() {
		names = append(names, in.name)
		if !strings.EqualFold(in.name, p.Service) {
			continue
		}
		if in.daemonAddr != p.daemonAddr() || !sameDir(in.stateDir, p.StateDir) {
			return fmt.Errorf("service %s exists but was not created by 'instance add' (address %s, folder %s): remove it by hand",
				in.name, in.daemonAddr, in.stateDir)
		}
		return nil
	}
	return fmt.Errorf("no network named %q (NetBird services found: %s)", p.Name, strings.Join(names, ", "))
}

func runSteps(out io.Writer, exe string, steps []planStep) error {
	for i, s := range steps {
		fmt.Fprintf(out, "\n[%d/%d] %s\n", i+1, len(steps), s.Desc)
		err := runStep(out, exe, s)
		switch {
		case err == nil:
		case s.IgnoreError:
			fmt.Fprintf(out, "  (ignored: %v)\n", err)
		default:
			return fmt.Errorf("step %d (%s): %w", i+1, s.Desc, err)
		}
	}
	return nil
}

func runStep(out io.Writer, exe string, s planStep) error {
	switch s.Kind {
	case stepRun:
		return runProgram(out, exe, s)
	case stepWait:
		return waitForDaemon(exe, s)
	case stepProfile:
		return prepareProfile(s)
	case stepShortcut:
		return createShortcut(s)
	case stepRemoveLnk:
		if err := os.Remove(s.Link); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	case stepRemoveDir:
		return removeInstanceDir(s.Dir)
	}
	return fmt.Errorf("unknown step kind %d", s.Kind)
}

// runProgram runs this program again with the arguments and environment of the step.
func runProgram(out io.Writer, exe string, s planStep) error {
	c := exec.Command(exe, s.Args...)
	c.Env = mergeEnv(os.Environ(), s.Env)

	if s.Stream {
		c.Stdout, c.Stderr = out, out
		return c.Run()
	}

	output, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, lastLines(string(output), 6))
	}
	return nil
}

// waitForDaemon waits until the daemon of the new service answers on its pipe.
func waitForDaemon(exe string, s planStep) error {
	deadline := time.Now().Add(daemonStartTimeout)
	var lastErr error
	for {
		c := exec.Command(exe, s.Args...)
		c.Env = mergeEnv(os.Environ(), s.Env)
		if output, err := c.CombinedOutput(); err == nil {
			return nil
		} else {
			lastErr = fmt.Errorf("%w: %s", err, lastLines(string(output), 3))
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("the daemon did not answer within %s: %w", daemonStartTimeout, lastErr)
		}
		time.Sleep(daemonStartInterval)
	}
}

// prepareProfile copies a profile into the new state folder and points it at the
// new interface and port, using NetBird's own config code so the file keeps its
// format and gets the same restricted permissions as any other.
func prepareProfile(s planStep) error {
	data, err := os.ReadFile(s.From)
	if err != nil {
		return fmt.Errorf("read %s: %w", s.From, err)
	}
	if err := os.MkdirAll(filepath.Dir(s.To), 0o700); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(s.To), err)
	}
	if err := os.WriteFile(s.To, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", s.To, err)
	}

	iface, port := s.Interface, s.Port
	if _, err := profilemanager.UpdateOrCreateConfig(profilemanager.ConfigInput{
		ConfigPath:    s.To,
		InterfaceName: &iface,
		WireguardPort: &port,
	}); err != nil {
		return fmt.Errorf("set interface and port in %s: %w", s.To, err)
	}
	return nil
}

// createShortcut writes the tray's startup shortcut with PowerShell, the only
// tool that makes .lnk files without extra dependencies.
func createShortcut(s planStep) error {
	if err := os.MkdirAll(filepath.Dir(s.Link), 0o755); err != nil {
		return err
	}

	script := fmt.Sprintf("$s=(New-Object -ComObject WScript.Shell).CreateShortcut(%s); $s.TargetPath=%s; $s.Arguments=%s; "+
		"$s.WorkingDirectory=%s; $s.Description=%s; $s.Save()",
		psQuote(s.Link), psQuote(s.Target), psQuote(s.Arguments), psQuote(filepath.Dir(s.Target)), psQuote("NetBird"))

	powershell := wincmd.System32(`WindowsPowerShell\v1.0\powershell`)
	if output, err := exec.Command(powershell, "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, lastLines(string(output), 4))
	}
	return nil
}

// removeInstanceDir deletes an instance's folder, and only a folder that "instance
// add" could have made: Netbird-<name> directly under ProgramData.
func removeInstanceDir(dir string) error {
	programData := os.Getenv("PROGRAMDATA")
	base := filepath.Base(dir)

	if programData == "" || !sameDir(filepath.Dir(dir), programData) ||
		!strings.HasPrefix(strings.ToLower(base), strings.ToLower(instanceDirPrefix)) || len(base) <= len(instanceDirPrefix) {
		return fmt.Errorf("refusing to delete %s: not an instance folder", dir)
	}
	return os.RemoveAll(dir)
}

// psQuote quotes a value for PowerShell. Names and paths are passed only as
// quoted strings, never as code.
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// mergeEnv returns base with the given KEY=VALUE entries replacing any entry of
// the same name (names are case insensitive on Windows).
func mergeEnv(base, overrides []string) []string {
	skip := map[string]bool{}
	for _, kv := range overrides {
		if k, _, ok := strings.Cut(kv, "="); ok {
			skip[strings.ToLower(k)] = true
		}
	}

	merged := make([]string, 0, len(base)+len(overrides))
	for _, kv := range base {
		if k, _, ok := strings.Cut(kv, "="); ok && skip[strings.ToLower(k)] {
			continue
		}
		merged = append(merged, kv)
	}
	return append(merged, overrides...)
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.TrimSpace(strings.Join(lines, " | "))
}
