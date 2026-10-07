package cmd

import (
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc/mgr"

	daddr "github.com/netbirdio/netbird/client/internal/daemonaddr"
)

const servicesRegistryRoot = `SYSTEM\CurrentControlSet\Services`

// installedInstances lists the installed NetBird services. A failure to read
// them is logged and yields what was found, so the guard never blocks an install
// because the service manager could not be read.
func installedInstances() []instanceInfo {
	m, err := mgr.Connect()
	if err != nil {
		log.Debugf("connect to the service manager to list NetBird instances: %v", err)
		return nil
	}
	defer func() { _ = m.Disconnect() }()

	names, err := m.ListServices()
	if err != nil {
		log.Debugf("list services: %v", err)
		return nil
	}

	var found []instanceInfo
	for _, name := range names {
		if info, ok := readInstance(m, name); ok {
			found = append(found, info)
		}
	}
	return found
}

func readInstance(m *mgr.Mgr, name string) (instanceInfo, bool) {
	s, err := m.OpenService(name)
	if err != nil {
		return instanceInfo{}, false
	}
	defer func() { _ = s.Close() }()

	cfg, err := s.Config()
	if err != nil {
		return instanceInfo{}, false
	}

	return parseInstance(name, cfg.BinaryPathName, serviceEnvironment(name))
}

// serviceEnvironment returns the environment a service is registered with.
func serviceEnvironment(name string) []string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, servicesRegistryRoot+`\`+name, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer func() { _ = k.Close() }()

	env, _, err := k.GetStringsValue("Environment")
	if err != nil {
		return nil
	}
	return env
}

// parseInstance reads the daemon address and state directory of a NetBird
// service from its command line and environment. It reports false for any
// service that is not "<exe> service run ...".
func parseInstance(name, binaryPath string, env []string) (instanceInfo, bool) {
	args, err := windows.DecomposeCommandLine(binaryPath)
	if err != nil {
		return instanceInfo{}, false
	}

	run := -1
	for i := 0; i+1 < len(args); i++ {
		if strings.EqualFold(args[i], "service") && strings.EqualFold(args[i+1], "run") {
			run = i
			break
		}
	}
	if run < 0 {
		return instanceInfo{}, false
	}

	info := instanceInfo{
		name:       name,
		daemonAddr: daddr.WindowsPipeAddr,
		stateDir:   filepath.Join(os.Getenv("PROGRAMDATA"), "Netbird"),
	}
	for i := run + 2; i+1 < len(args); i++ {
		if args[i] == "--daemon-addr" {
			info.daemonAddr = args[i+1]
		}
	}
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "NB_STATE_DIR="); ok && v != "" {
			info.stateDir = v
		}
	}
	return info, true
}
