package device

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
)

var (
	claimMu sync.Mutex
	// claimed holds the mutex handle of every interface name this process owns.
	claimed = map[string]windows.Handle{}
)

func claimMutexName(ifaceName string) string {
	return `Global\NetbirdInterface_` + strings.ToLower(ifaceName)
}

// claimInterface reserves the interface name for this process. Two NetBird
// daemons given the same name would also derive the same adapter GUID, and
// Wintun then opens the adapter of the first instead of creating a second one.
// The reservation is a named mutex, which the system releases when the process
// ends, so a crashed daemon never leaves a stale claim behind.
//
// Claiming a name this process already owns succeeds, since the engine creates
// the device again on every reconnect.
func claimInterface(ifaceName string) error {
	claimMu.Lock()
	defer claimMu.Unlock()

	key := strings.ToLower(ifaceName)
	if _, ok := claimed[key]; ok {
		return nil
	}

	name, err := windows.UTF16PtrFromString(claimMutexName(ifaceName))
	if err != nil {
		log.Warnf("skip claiming interface %s: %v", ifaceName, err)
		return nil
	}

	handle, err := windows.CreateMutex(nil, false, name)
	switch {
	case errors.Is(err, windows.ERROR_ALREADY_EXISTS):
		_ = windows.CloseHandle(handle)
		return fmt.Errorf("interface %s is already used by another NetBird instance on this machine: "+
			"give this one its own name with 'netbird up --interface-name <name>' (names start with wt)", ifaceName)
	case err != nil:
		// An unexpected failure must not stop the default instance from starting.
		log.Warnf("skip claiming interface %s: %v", ifaceName, err)
		return nil
	}

	claimed[key] = handle
	return nil
}

// releaseInterface gives the name back, so another instance may use it.
func releaseInterface(ifaceName string) {
	claimMu.Lock()
	defer claimMu.Unlock()

	key := strings.ToLower(ifaceName)
	if handle, ok := claimed[key]; ok {
		_ = windows.CloseHandle(handle)
		delete(claimed, key)
	}
}
