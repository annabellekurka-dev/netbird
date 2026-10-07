//go:build !ios && !android

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/netbirdio/netbird/client/configs"
	daddr "github.com/netbirdio/netbird/client/internal/daemonaddr"
)

const defaultServiceNameFold = "netbird"

// instanceInfo is what one installed NetBird service is configured with.
type instanceInfo struct {
	name       string
	daemonAddr string
	stateDir   string
}

// sameDir reports whether two directory paths name the same folder, ignoring
// case and trailing separators as Windows does.
func sameDir(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// checkInstanceInstall refuses an install of a service other than the default
// one when it would share state with another instance. The installer writes
// service.json into the state directory of its own process, so an instance
// installed without a state directory of its own would overwrite the saved
// parameters of the default instance.
//
// self is the service about to be installed, envStateDir the NB_STATE_DIR it
// will run with, defaultStateDir the folder of the default instance, and others
// the services already installed.
func checkInstanceInstall(self instanceInfo, envStateDir, defaultStateDir string, others []instanceInfo, logFiles []string, defaultLogFile string) error {
	if strings.EqualFold(self.name, defaultServiceNameFold) {
		return nil
	}

	// The default log file lives in the default state directory whatever
	// NB_STATE_DIR says, so two daemons would write to the same file.
	for _, f := range logFiles {
		if strings.EqualFold(filepath.Clean(f), filepath.Clean(defaultLogFile)) {
			return fmt.Errorf("service %q would write to the default log file %s, which the default instance uses: "+
				"pass --log-file <a file in its own state directory>", self.name, defaultLogFile)
		}
	}

	if sameDir(self.stateDir, defaultStateDir) {
		return fmt.Errorf("service %q would use the default state directory %s, which belongs to the default instance and "+
			"would have its saved service parameters overwritten: set NB_STATE_DIR to a folder of its own in this shell and "+
			"pass the same folder with --service-env NB_STATE_DIR=<folder>", self.name, defaultStateDir)
	}

	if envStateDir == "" || !sameDir(envStateDir, self.stateDir) {
		return fmt.Errorf("service %q must run with NB_STATE_DIR=%s, the folder this command writes to: pass --service-env NB_STATE_DIR=%s",
			self.name, self.stateDir, self.stateDir)
	}

	if self.daemonAddr == daddr.WindowsPipeAddr {
		return fmt.Errorf("service %q would serve the default daemon address %s: pass --daemon-addr npipe://<name> with a name of its own",
			self.name, daddr.WindowsPipeAddr)
	}

	for _, other := range others {
		if strings.EqualFold(other.name, self.name) {
			continue
		}
		if self.daemonAddr == other.daemonAddr {
			return fmt.Errorf("service %q and service %q would serve the same daemon address %s", self.name, other.name, self.daemonAddr)
		}
		if sameDir(self.stateDir, other.stateDir) {
			return fmt.Errorf("service %q and service %q would share the state directory %s", self.name, other.name, self.stateDir)
		}
	}

	return nil
}

// checkInstanceInstallConflicts runs checkInstanceInstall for the service the
// install or reconfigure command is about to register. It only applies on
// Windows, where running several instances side by side is supported.
func checkInstanceInstallConflicts() error {
	if runtime.GOOS != "windows" {
		return nil
	}

	var envStateDir string
	if env, err := parseServiceEnvVars(serviceEnvVars); err == nil {
		envStateDir = env["NB_STATE_DIR"]
	}

	self := instanceInfo{name: serviceName, daemonAddr: daemonAddr, stateDir: configs.StateDir}
	defaultStateDir := filepath.Join(os.Getenv("PROGRAMDATA"), "Netbird")

	return checkInstanceInstall(self, envStateDir, defaultStateDir, installedInstances(), logFiles, defaultLogFile)
}
