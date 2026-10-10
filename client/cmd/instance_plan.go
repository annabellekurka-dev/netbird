//go:build !ios && !android

package cmd

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	instanceServicePrefix = "NetBird-"
	instancePipePrefix    = "netbird-"
	instanceDirPrefix     = "Netbird-"

	firstInstancePort = 51820
	maxInstanceIndex  = 99
)

var (
	instanceNamePattern  = regexp.MustCompile(`^[a-z][a-z0-9-]{0,19}$`)
	interfaceNamePattern = regexp.MustCompile(`^wt[a-z0-9]{1,8}$`)
)

// addOptions is what the user asked for with "instance add".
type addOptions struct {
	Name          string
	SetupKey      string
	ManagementURL string
	// FromProfile is a profile file to start the instance from, so it comes up as
	// that identity instead of registering a new peer. "default" means the profile
	// of the default instance.
	FromProfile string
	Interface   string
	Port        int
	NoShortcut  bool
}

// takenResources is what the other instances on the machine already use. All
// names are lower case.
type takenResources struct {
	Services   map[string]bool
	Pipes      map[string]bool
	Dirs       map[string]bool
	Interfaces map[string]bool
	Ports      map[int]bool
}

// instancePlan is every name an extra instance needs, derived from its name.
type instancePlan struct {
	Name      string
	Service   string
	Pipe      string
	StateDir  string
	LogFile   string
	Interface string
	Port      int
}

func (p instancePlan) daemonAddr() string { return "npipe://" + p.Pipe }

func (p instancePlan) env() []string { return []string{"NB_STATE_DIR=" + p.StateDir} }

// trayArguments are the arguments of the instance's tray. The badge number is the
// number of the interface (wt1 is 1), so each tray gets a dot of its own colour.
func (p instancePlan) trayArguments() string {
	args := "--daemon-addr " + p.daemonAddr()
	if n, err := strconv.Atoi(strings.TrimPrefix(p.Interface, "wt")); err == nil && n > 0 {
		args += " --badge " + strconv.Itoa(n)
	}
	return args
}

// deriveInstance returns the service, pipe and folders for an instance name. It
// leaves the interface and port empty: those depend on what else is running.
func deriveInstance(name, programData string) (instancePlan, error) {
	if !instanceNamePattern.MatchString(name) {
		return instancePlan{}, fmt.Errorf("instance name %q must start with a letter and hold only lower-case letters, digits and '-' (up to 20 characters)", name)
	}
	if name == "netbird" || name == "default" {
		return instancePlan{}, fmt.Errorf("instance name %q is reserved", name)
	}

	dir := filepath.Join(programData, instanceDirPrefix+name)
	return instancePlan{
		Name:     name,
		Service:  instanceServicePrefix + name,
		Pipe:     instancePipePrefix + name,
		StateDir: dir,
		LogFile:  filepath.Join(dir, "client.log"),
	}, nil
}

// planInstance derives everything for a new instance and picks the first free
// interface and port, refusing names that another instance already uses.
func planInstance(o addOptions, programData string, t takenResources) (instancePlan, error) {
	p, err := deriveInstance(o.Name, programData)
	if err != nil {
		return instancePlan{}, err
	}

	switch {
	case t.Services[strings.ToLower(p.Service)]:
		return instancePlan{}, fmt.Errorf("service %s already exists: use 'instance remove %s' first", p.Service, p.Name)
	case t.Pipes[strings.ToLower(p.Pipe)]:
		return instancePlan{}, fmt.Errorf("daemon address %s is already used by another instance", p.daemonAddr())
	case t.Dirs[strings.ToLower(p.StateDir)]:
		return instancePlan{}, fmt.Errorf("state folder %s is already used by another instance", p.StateDir)
	}

	if p.Interface, err = chooseInterface(o.Interface, t.Interfaces); err != nil {
		return instancePlan{}, err
	}
	if p.Port, err = choosePort(o.Port, t.Ports); err != nil {
		return instancePlan{}, err
	}
	return p, nil
}

func chooseInterface(wanted string, taken map[string]bool) (string, error) {
	if wanted != "" {
		wanted = strings.ToLower(wanted)
		if !interfaceNamePattern.MatchString(wanted) {
			return "", fmt.Errorf("interface name %q must start with 'wt' (letters and digits after it)", wanted)
		}
		if taken[wanted] {
			return "", fmt.Errorf("interface %s is already used by another instance", wanted)
		}
		return wanted, nil
	}

	for i := 1; i <= maxInstanceIndex; i++ {
		if name := "wt" + strconv.Itoa(i); !taken[name] {
			return name, nil
		}
	}
	return "", errors.New("no free interface name wt1..wt99")
}

func choosePort(wanted int, taken map[int]bool) (int, error) {
	if wanted != 0 {
		if wanted < 1024 || wanted > 65535 {
			return 0, fmt.Errorf("WireGuard port %d must be between 1024 and 65535", wanted)
		}
		if taken[wanted] {
			return 0, fmt.Errorf("WireGuard port %d is already used by another instance", wanted)
		}
		return wanted, nil
	}

	for i := 1; i <= maxInstanceIndex; i++ {
		if port := firstInstancePort + i; !taken[port] {
			return port, nil
		}
	}
	return 0, errors.New("no free WireGuard port")
}

type stepKind int

const (
	stepRun       stepKind = iota // run this program with Args
	stepProfile                   // copy a profile into the state folder and set its interface and port
	stepWait                      // wait until the daemon answers on its pipe
	stepShortcut                  // create the tray's startup shortcut
	stepRemoveLnk                 // delete that shortcut
	stepRemoveDir                 // delete the state folder
)

// planStep is one thing "instance add" or "instance remove" does.
type planStep struct {
	Kind stepKind
	Desc string

	// stepRun and stepWait
	Args        []string
	Env         []string
	Stream      bool // show the output while it runs: "up" may print a login link
	IgnoreError bool

	// stepProfile
	From, To, Interface string
	Port                int

	// stepShortcut and stepRemoveLnk
	Link, Target, Arguments string

	// stepRemoveDir
	Dir string
}

// shortcutEnv is where the tray shortcut goes and what it starts.
type shortcutEnv struct {
	StartupDir string
	UIExe      string
}

func (e shortcutEnv) link(name string) string {
	return filepath.Join(e.StartupDir, "NetBird ("+name+").lnk")
}

// addSteps lists what "instance add" does, in order. profileSource is the file to
// start from, or "" to register a new peer or sign in through the browser.
func addSteps(p instancePlan, o addOptions, profileSource string, sc shortcutEnv) []planStep {
	steps := []planStep{{
		Kind: stepRun,
		Desc: "install the service " + p.Service,
		Args: []string{"service", "install", "-s", p.Service, "--daemon-addr", p.daemonAddr(),
			"--service-env", "NB_STATE_DIR=" + p.StateDir, "--log-file", p.LogFile},
		Env: p.env(),
	}}

	if profileSource != "" {
		steps = append(steps, planStep{
			Kind: stepProfile,
			Desc: fmt.Sprintf("copy the profile into %s and set its interface to %s, port %d", p.StateDir, p.Interface, p.Port),
			From: profileSource, To: filepath.Join(p.StateDir, "default.json"), Interface: p.Interface, Port: p.Port,
		})
	}

	up := []string{"up", "--daemon-addr", p.daemonAddr(), "--interface-name", p.Interface, "--wireguard-port", strconv.Itoa(p.Port)}
	if o.SetupKey != "" {
		up = append(up, "--setup-key", o.SetupKey)
	}
	if o.ManagementURL != "" {
		up = append(up, "--management-url", o.ManagementURL)
	}

	steps = append(steps,
		planStep{Kind: stepRun, Desc: "start the service", Args: []string{"service", "start", "-s", p.Service}, Env: p.env()},
		planStep{Kind: stepWait, Desc: "wait for the daemon", Args: []string{"status", "--daemon-addr", p.daemonAddr()}, Env: p.env()},
		planStep{Kind: stepRun, Desc: "connect (a login link may appear)", Args: up, Env: p.env(), Stream: true},
	)

	if !o.NoShortcut && sc.UIExe != "" {
		steps = append(steps, planStep{
			Kind: stepShortcut,
			Desc: "add the tray to the startup folder",
			Link: sc.link(p.Name), Target: sc.UIExe, Arguments: p.trayArguments(),
		})
	}
	return steps
}

// removeSteps lists what "instance remove" does. Stopping is allowed to fail:
// the point is to get rid of whatever is left.
func removeSteps(p instancePlan, keepData bool, sc shortcutEnv) []planStep {
	steps := []planStep{
		{Kind: stepRun, Desc: "disconnect", Args: []string{"down", "--daemon-addr", p.daemonAddr()}, Env: p.env(), IgnoreError: true},
		{Kind: stepRun, Desc: "stop the service", Args: []string{"service", "stop", "-s", p.Service}, Env: p.env(), IgnoreError: true},
		{Kind: stepRun, Desc: "remove the service", Args: []string{"service", "uninstall", "-s", p.Service}, Env: p.env()},
		{Kind: stepRemoveLnk, Desc: "remove the tray from the startup folder", Link: sc.link(p.Name)},
	}
	if !keepData {
		steps = append(steps, planStep{Kind: stepRemoveDir, Desc: "delete " + p.StateDir + " (keys, profile, logs)", Dir: p.StateDir})
	}
	return steps
}

// display shows a step for the plan and the dry run, with secrets hidden.
func (s planStep) display() string {
	if s.Kind != stepRun && s.Kind != stepWait {
		return s.Desc
	}

	args := append([]string(nil), s.Args...)
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--setup-key" {
			args[i+1] = "<hidden>"
		}
	}
	return s.Desc + ":  netbird " + strings.Join(args, " ")
}
