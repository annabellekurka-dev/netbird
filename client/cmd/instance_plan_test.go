//go:build !ios && !android

package cmd

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testProgramData = `C:\ProgramData`

func noneTaken() takenResources {
	return takenResources{
		Services:   map[string]bool{},
		Pipes:      map[string]bool{},
		Dirs:       map[string]bool{},
		Interfaces: map[string]bool{},
		Ports:      map[int]bool{},
	}
}

// taken describes a machine that already has the default instance.
func defaultTaken() takenResources {
	t := noneTaken()
	t.Services["netbird"] = true
	t.Pipes["netbird"] = true
	t.Dirs[`c:\programdata\netbird`] = true
	t.Interfaces["wt0"] = true
	t.Ports[51820] = true
	return t
}

func TestDeriveInstanceNames(t *testing.T) {
	p, err := deriveInstance("private", testProgramData)
	require.NoError(t, err)

	assert.Equal(t, "NetBird-private", p.Service)
	assert.Equal(t, "netbird-private", p.Pipe)
	assert.Equal(t, "npipe://netbird-private", p.daemonAddr())
	assert.Equal(t, filepath.Join(testProgramData, "Netbird-private"), p.StateDir)
	assert.Equal(t, filepath.Join(testProgramData, "Netbird-private", "client.log"), p.LogFile)
	assert.Equal(t, []string{"NB_STATE_DIR=" + p.StateDir}, p.env())
}

func TestDeriveInstanceRejectsBadNames(t *testing.T) {
	for _, name := range []string{"", "Private", "1work", "a b", "a_b", "work/../x", `a\b`, "x'y", "netbird", "default", "averyveryverylongname1"} {
		_, err := deriveInstance(name, testProgramData)
		assert.Error(t, err, "name %q", name)
	}
	for _, name := range []string{"private", "work2", "a", "my-network"} {
		_, err := deriveInstance(name, testProgramData)
		assert.NoError(t, err, "name %q", name)
	}
}

func TestPlanPicksFirstFreeInterfaceAndPort(t *testing.T) {
	p, err := planInstance(addOptions{Name: "private"}, testProgramData, defaultTaken())
	require.NoError(t, err)
	assert.Equal(t, "wt1", p.Interface)
	assert.Equal(t, 51821, p.Port)

	taken := defaultTaken()
	taken.Interfaces["wt1"] = true
	taken.Ports[51821] = true
	p, err = planInstance(addOptions{Name: "second"}, testProgramData, taken)
	require.NoError(t, err)
	assert.Equal(t, "wt2", p.Interface, "a used interface is skipped")
	assert.Equal(t, 51822, p.Port, "a used port is skipped")
}

func TestPlanHonoursExplicitChoices(t *testing.T) {
	p, err := planInstance(addOptions{Name: "private", Interface: "WT5", Port: 51900}, testProgramData, defaultTaken())
	require.NoError(t, err)
	assert.Equal(t, "wt5", p.Interface)
	assert.Equal(t, 51900, p.Port)
}

func TestPlanRefusesClashes(t *testing.T) {
	tests := []struct {
		name    string
		opts    addOptions
		taken   func() takenResources
		wantErr string
	}{
		{"service exists", addOptions{Name: "private"}, func() takenResources {
			t := defaultTaken()
			t.Services["netbird-private"] = true
			return t
		}, "already exists"},
		{"pipe used", addOptions{Name: "private"}, func() takenResources {
			t := defaultTaken()
			t.Pipes["netbird-private"] = true
			return t
		}, "daemon address"},
		{"folder used", addOptions{Name: "private"}, func() takenResources {
			t := defaultTaken()
			t.Dirs[`c:\programdata\netbird-private`] = true
			return t
		}, "state folder"},
		{"interface used", addOptions{Name: "private", Interface: "wt0"}, defaultTaken, "interface wt0"},
		{"interface name without wt", addOptions{Name: "private", Interface: "eth1"}, defaultTaken, "must start with 'wt'"},
		{"port used", addOptions{Name: "private", Port: 51820}, defaultTaken, "port 51820"},
		{"port out of range", addOptions{Name: "private", Port: 80}, defaultTaken, "between 1024 and 65535"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := planInstance(tc.opts, testProgramData, tc.taken())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestAddStepsWithSetupKey(t *testing.T) {
	o := addOptions{Name: "private", SetupKey: "SECRET-KEY", ManagementURL: "https://mgmt.example.com:443"}
	p, err := planInstance(o, testProgramData, defaultTaken())
	require.NoError(t, err)

	steps := addSteps(p, o, "", shortcutEnv{StartupDir: `C:\Startup`, UIExe: `C:\NB\netbird-ui.exe`})
	require.Len(t, steps, 5)

	assert.Equal(t, []string{"service", "install", "-s", "NetBird-private", "--daemon-addr", "npipe://netbird-private",
		"--service-env", `NB_STATE_DIR=` + p.StateDir, "--log-file", p.LogFile}, steps[0].Args)
	assert.Equal(t, []string{"NB_STATE_DIR=" + p.StateDir}, steps[0].Env, "the installer writes service.json into this folder")

	assert.Equal(t, stepRun, steps[1].Kind)
	assert.Equal(t, []string{"service", "start", "-s", "NetBird-private"}, steps[1].Args)
	assert.Equal(t, stepWait, steps[2].Kind)

	up := steps[3]
	assert.True(t, up.Stream, "a login link must be shown while it runs")
	assert.Equal(t, []string{"up", "--daemon-addr", "npipe://netbird-private", "--interface-name", "wt1", "--wireguard-port", "51821",
		"--setup-key", "SECRET-KEY", "--management-url", "https://mgmt.example.com:443"}, up.Args)
	assert.Equal(t, []string{"NB_STATE_DIR=" + p.StateDir}, up.Env)

	assert.Equal(t, stepShortcut, steps[4].Kind)
	assert.Equal(t, `C:\Startup\NetBird (private).lnk`, steps[4].Link)
	assert.Equal(t, "--daemon-addr npipe://netbird-private --badge 1", steps[4].Arguments, "the badge number is the interface number")
}

func TestTrayArguments(t *testing.T) {
	p := instancePlan{Pipe: "netbird-x", Interface: "wt3"}
	assert.Equal(t, "--daemon-addr npipe://netbird-x --badge 3", p.trayArguments())

	p.Interface = "wtlab"
	assert.Equal(t, "--daemon-addr npipe://netbird-x", p.trayArguments(), "no number, no badge argument")
}

func TestAddStepsWithProfile(t *testing.T) {
	o := addOptions{Name: "work"}
	p, err := planInstance(o, testProgramData, defaultTaken())
	require.NoError(t, err)

	steps := addSteps(p, o, `C:\ProgramData\Netbird\default.json`, shortcutEnv{})
	require.Len(t, steps, 5, "install, profile, start, wait, connect; no shortcut without a UI path")

	assert.Equal(t, stepRun, steps[0].Kind, "the service is installed first so its folder exists")
	require.Equal(t, stepProfile, steps[1].Kind)
	assert.Equal(t, `C:\ProgramData\Netbird\default.json`, steps[1].From)
	assert.Equal(t, filepath.Join(p.StateDir, "default.json"), steps[1].To)
	assert.Equal(t, "wt1", steps[1].Interface)
	assert.Equal(t, 51821, steps[1].Port)
	assert.Equal(t, []string{"service", "start", "-s", "NetBird-work"}, steps[2].Args, "the profile is in place before the service starts")
	assert.Equal(t, stepWait, steps[3].Kind)
	assert.NotContains(t, steps[4].Args, "--setup-key", "the profile already holds the identity")
}

func TestAddStepsWithoutShortcut(t *testing.T) {
	o := addOptions{Name: "private", NoShortcut: true}
	p, err := planInstance(o, testProgramData, defaultTaken())
	require.NoError(t, err)

	for _, s := range addSteps(p, o, "", shortcutEnv{StartupDir: `C:\Startup`, UIExe: `C:\NB\netbird-ui.exe`}) {
		assert.NotEqual(t, stepShortcut, s.Kind)
	}
}

func TestDisplayHidesTheSetupKey(t *testing.T) {
	o := addOptions{Name: "private", SetupKey: "SECRET-KEY"}
	p, err := planInstance(o, testProgramData, defaultTaken())
	require.NoError(t, err)

	for _, s := range addSteps(p, o, "", shortcutEnv{}) {
		assert.NotContains(t, s.display(), "SECRET-KEY")
	}
	assert.Contains(t, addSteps(p, o, "", shortcutEnv{})[3].display(), "--setup-key <hidden>")
}

func TestRemoveSteps(t *testing.T) {
	p, err := deriveInstance("private", testProgramData)
	require.NoError(t, err)
	sc := shortcutEnv{StartupDir: `C:\Startup`}

	steps := removeSteps(p, false, sc)
	require.Len(t, steps, 5)
	assert.True(t, steps[0].IgnoreError, "disconnecting may fail when the daemon is already gone")
	assert.True(t, steps[1].IgnoreError)
	assert.False(t, steps[2].IgnoreError, "removing the service must succeed or be reported")
	assert.Equal(t, []string{"service", "uninstall", "-s", "NetBird-private"}, steps[2].Args)
	assert.Equal(t, stepRemoveLnk, steps[3].Kind)
	assert.Equal(t, stepRemoveDir, steps[4].Kind)
	assert.Equal(t, p.StateDir, steps[4].Dir)

	assert.Len(t, removeSteps(p, true, sc), 4, "--keep-data leaves the folder")
}
