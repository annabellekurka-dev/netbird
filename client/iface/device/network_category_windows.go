package device

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/netbirdio/netbird/client/internal/wincmd"
)

const (
	networkCategoryAttempts = 15
	networkCategoryInterval = 2 * time.Second
	networkCategoryTimeout  = 20 * time.Second
)

// safeAliasPattern limits the interface names that reach the PowerShell command
// line. NetBird interface names are short and plain, so anything else is left
// alone rather than quoted.
var safeAliasPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,15}$`)

// networkCategoryScript sets the connection profile of the interface to
// Private unless it already is.
func networkCategoryScript(alias string) string {
	return fmt.Sprintf(
		"$p = Get-NetConnectionProfile -InterfaceAlias '%[1]s' -ErrorAction Stop; "+
			"if ($p.NetworkCategory -ne 'Private') { "+
			"Set-NetConnectionProfile -InterfaceAlias '%[1]s' -NetworkCategory Private -ErrorAction Stop }",
		alias)
}

// ensurePrivateNetwork puts a new interface on the Private firewall profile.
// Windows classifies a new adapter as Public, and where policy sets
// AllowLocalFirewallRules to false for that profile the allow rule NetBird adds
// is ignored, so inbound traffic such as ping is blocked by default. Windows
// only classifies the adapter a few seconds after it appears, so the call is
// retried for a while, and gives up quietly once the interface is closed.
func ensurePrivateNetwork(ifaceName string) {
	if !safeAliasPattern.MatchString(ifaceName) {
		log.Debugf("not setting the network category of interface %q: unexpected name", ifaceName)
		return
	}

	var lastErr error
	for i := 0; i < networkCategoryAttempts; i++ {
		time.Sleep(networkCategoryInterval)

		if !isClaimed(ifaceName) {
			return
		}

		if lastErr = runNetworkCategory(ifaceName); lastErr == nil {
			log.Infof("interface %s is on the Private network profile", ifaceName)
			return
		}
	}

	log.Warnf("could not put interface %s on the Private network profile, inbound traffic may be blocked if policy "+
		"ignores local firewall rules for the Public profile (see security event 5152): %v", ifaceName, lastErr)
}

func isClaimed(ifaceName string) bool {
	claimMu.Lock()
	defer claimMu.Unlock()

	_, ok := claimed[strings.ToLower(ifaceName)]
	return ok
}

func runNetworkCategory(ifaceName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), networkCategoryTimeout)
	defer cancel()

	powershell := wincmd.System32(`WindowsPowerShell\v1.0\powershell`)
	cmd := exec.CommandContext(ctx, powershell, "-NoProfile", "-NonInteractive", "-Command", networkCategoryScript(ifaceName))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
