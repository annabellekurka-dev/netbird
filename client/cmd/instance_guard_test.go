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
)

func TestCheckInstanceInstall(t *testing.T) {
	good := instanceInfo{name: "NetBird-private", daemonAddr: "npipe://netbird-private", stateDir: testOwnDir}
	defaultInstance := instanceInfo{name: "Netbird", daemonAddr: "npipe://netbird", stateDir: testDefaultDir}

	tests := []struct {
		name    string
		self    instanceInfo
		env     string
		others  []instanceInfo
		wantErr string
	}{
		{"default service is never checked", instanceInfo{name: "Netbird", daemonAddr: "npipe://netbird", stateDir: testDefaultDir}, "", nil, ""},
		{"default service name is case insensitive", instanceInfo{name: "NETBIRD", daemonAddr: "npipe://netbird", stateDir: testDefaultDir}, "", nil, ""},
		{"complete second instance", good, testOwnDir, []instanceInfo{defaultInstance}, ""},
		{"paths compare case and slash insensitively", good, `c:\programdata\netbird-private\`, []instanceInfo{defaultInstance}, ""},
		{
			"default state dir would be overwritten",
			instanceInfo{name: "NetBird-private", daemonAddr: "npipe://netbird-private", stateDir: testDefaultDir},
			testDefaultDir, nil, "default state directory",
		},
		{"state dir not passed to the service", good, "", nil, "must run with NB_STATE_DIR"},
		{"service runs with another state dir", good, `C:\other`, nil, "must run with NB_STATE_DIR"},
		{
			"default daemon address",
			instanceInfo{name: "NetBird-private", daemonAddr: "npipe://netbird", stateDir: testOwnDir},
			testOwnDir, nil, "default daemon address",
		},
		{
			"daemon address used by another instance",
			good, testOwnDir,
			[]instanceInfo{{name: "Third", daemonAddr: "npipe://netbird-private", stateDir: `C:\third`}},
			"same daemon address",
		},
		{
			"state dir used by another instance",
			good, testOwnDir,
			[]instanceInfo{{name: "Third", daemonAddr: "npipe://third", stateDir: testOwnDir}},
			"share the state directory",
		},
		{"reinstalling itself is not a conflict", good, testOwnDir, []instanceInfo{good}, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := checkInstanceInstall(tc.self, tc.env, testDefaultDir, tc.others)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}
