//go:build !ios && !android

package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testDefaultDir = `C:\ProgramData\Netbird`
	testOwnDir     = `C:\ProgramData\Netbird-private`
	testDefaultLog = `C:\ProgramData\Netbird\client.log`
	testOwnLog     = `C:\ProgramData\Netbird-private\client.log`
)

func TestCheckInstanceInstall(t *testing.T) {
	good := instanceInfo{name: "NetBird-private", daemonAddr: "npipe://netbird-private", stateDir: testOwnDir}
	defaultInstance := instanceInfo{name: "Netbird", daemonAddr: "npipe://netbird", stateDir: testDefaultDir}
	ownLog := []string{testOwnLog}

	tests := []struct {
		name    string
		self    instanceInfo
		env     string
		others  []instanceInfo
		logs    []string
		wantErr string
	}{
		{"default service is never checked", defaultInstance, "", nil, []string{testDefaultLog}, ""},
		{"default service name is case insensitive", instanceInfo{name: "NETBIRD", daemonAddr: "npipe://netbird", stateDir: testDefaultDir}, "", nil, []string{testDefaultLog}, ""},
		{"complete second instance", good, testOwnDir, []instanceInfo{defaultInstance}, ownLog, ""},
		{"console logging is fine", good, testOwnDir, []instanceInfo{defaultInstance}, []string{"console", testOwnLog}, ""},
		{"paths compare case and slash insensitively", good, `c:\programdata\netbird-private\`, []instanceInfo{defaultInstance}, ownLog, ""},
		{
			"default state dir would be overwritten",
			instanceInfo{name: "NetBird-private", daemonAddr: "npipe://netbird-private", stateDir: testDefaultDir},
			testDefaultDir, nil, ownLog, "default state directory",
		},
		{"state dir not passed to the service", good, "", nil, ownLog, "must run with NB_STATE_DIR"},
		{"service runs with another state dir", good, `C:\other`, nil, ownLog, "must run with NB_STATE_DIR"},
		{
			"default daemon address",
			instanceInfo{name: "NetBird-private", daemonAddr: "npipe://netbird", stateDir: testOwnDir},
			testOwnDir, nil, ownLog, "default daemon address",
		},
		{
			"daemon address used by another instance",
			good, testOwnDir,
			[]instanceInfo{{name: "Third", daemonAddr: "npipe://netbird-private", stateDir: `C:\third`}},
			ownLog, "same daemon address",
		},
		{
			"state dir used by another instance",
			good, testOwnDir,
			[]instanceInfo{{name: "Third", daemonAddr: "npipe://third", stateDir: testOwnDir}},
			ownLog, "share the state directory",
		},
		{"reinstalling itself is not a conflict", good, testOwnDir, []instanceInfo{good}, ownLog, ""},
		{"default log file", good, testOwnDir, nil, []string{testDefaultLog}, "default log file"},
		{"default log file among others", good, testOwnDir, nil, []string{"console", `c:\programdata\NETBIRD\client.log`}, "default log file"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := checkInstanceInstall(tc.self, tc.env, testDefaultDir, tc.others, tc.logs, testDefaultLog)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}
