package device

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestClaimInterface(t *testing.T) {
	name := "wtclaimtest1"
	t.Cleanup(func() { releaseInterface(name) })

	require.NoError(t, claimInterface(name))
	assert.NoError(t, claimInterface(name), "the owner may claim again, as it does on every reconnect")
	assert.NoError(t, claimInterface("WTCLAIMTEST1"), "names are case insensitive")

	releaseInterface(name)
	require.NoError(t, claimInterface(name), "a released name can be claimed again")
}

// A holder outside this process stands for another daemon: the claim must fail
// with a message that tells the user what to do.
func TestClaimInterfaceHeldElsewhere(t *testing.T) {
	name := "wtclaimtest2"

	ptr, err := windows.UTF16PtrFromString(claimMutexName(name))
	require.NoError(t, err)
	holder, err := windows.CreateMutex(nil, false, ptr)
	if err != nil {
		t.Skipf("cannot create a global mutex here: %v", err)
	}
	t.Cleanup(func() { _ = windows.CloseHandle(holder) })

	err = claimInterface(name)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--interface-name")

	_ = windows.CloseHandle(holder)
	holder = 0
	assert.NoError(t, claimInterface(name), "claimable once the other holder is gone")
	releaseInterface(name)
}
