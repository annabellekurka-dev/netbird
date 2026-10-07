//go:build !android && !ios && !freebsd && !js

package main

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInstanceSuffix(t *testing.T) {
	assert.Equal(t, "", instanceSuffix(""))
	assert.Equal(t, "", instanceSuffix(DaemonAddr()), "the default daemon keeps the default identity")
	assert.Equal(t, "netbird-private", instanceSuffix("npipe://netbird-private"))
	assert.Equal(t, "netbird-private", instanceSuffix("npipe://NetBird_Private"))
	assert.Equal(t, "127-0-0-1-41732", instanceSuffix("tcp://127.0.0.1:41732"))
	assert.Equal(t, "tmp-x-sock", instanceSuffix("unix:///tmp/x.sock"))
	assert.Equal(t, "", instanceSuffix("npipe://---"))
	assert.Len(t, instanceSuffix("npipe://"+string(make([]byte, 0))+"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), 40)
}

func TestUniqueID(t *testing.T) {
	assert.Equal(t, "io.netbird.ui", uniqueID(""))
	assert.Equal(t, "io.netbird.ui.netbird-private", uniqueID("netbird-private"))
}

func TestWithInstanceLabel(t *testing.T) {
	assert.Equal(t, "NetBird", withInstanceLabel("NetBird", ""))
	assert.Equal(t, "NetBird (netbird-private)", withInstanceLabel("NetBird", "netbird-private"))
}

func TestFlagInstanceSuffixWithoutFlag(t *testing.T) {
	if flag.Lookup("daemon-addr") != nil {
		t.Skip("the flag is registered by another test in this process")
	}
	assert.Equal(t, "", flagInstanceSuffix())
}
