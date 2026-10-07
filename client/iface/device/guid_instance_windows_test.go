package device

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestInstanceGUIDString(t *testing.T) {
	assert.Empty(t, instanceGUIDString(""))
	assert.Empty(t, instanceGUIDString("wt0"))
	assert.Empty(t, instanceGUIDString("WT0"))

	wt1 := instanceGUIDString("wt1")
	require.NotEmpty(t, wt1)
	assert.Equal(t, wt1, instanceGUIDString("wt1"), "same name must give the same GUID")
	assert.Equal(t, wt1, instanceGUIDString("WT1"), "names are case insensitive")
	assert.NotEqual(t, wt1, instanceGUIDString("wt2"))
	assert.NotEqual(t, defaultWindowsGUIDSTring, wt1)

	_, err := windows.GUIDFromString(wt1)
	assert.NoError(t, err, "must parse as a Windows GUID")
}

// The default interface must keep the GUID older versions created, otherwise an
// existing adapter would no longer be found.
func TestGetGUIDDefaultUnchanged(t *testing.T) {
	want, err := windows.GUIDFromString(defaultWindowsGUIDSTring)
	require.NoError(t, err)

	for _, name := range []string{"", "wt0"} {
		got, err := getGUID(name)
		require.NoError(t, err)
		assert.Equal(t, want, got, "interface %q", name)
	}
}

func TestGetGUIDCustomOverrides(t *testing.T) {
	custom := "{11111111-2222-3333-4444-555555555555}"
	old := CustomWindowsGUIDString
	CustomWindowsGUIDString = custom
	t.Cleanup(func() { CustomWindowsGUIDString = old })

	want, err := windows.GUIDFromString(custom)
	require.NoError(t, err)

	got, err := getGUID("wt1")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
