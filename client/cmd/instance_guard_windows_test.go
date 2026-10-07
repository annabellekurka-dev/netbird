package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseInstance(t *testing.T) {
	t.Setenv("PROGRAMDATA", `C:\ProgramData`)

	t.Run("default install", func(t *testing.T) {
		info, ok := parseInstance("Netbird",
			`"C:\Program Files\Netbird\netbird.exe" service run --log-level info --daemon-addr npipe://netbird`, nil)
		assert.True(t, ok)
		assert.Equal(t, "npipe://netbird", info.daemonAddr)
		assert.Equal(t, `C:\ProgramData\Netbird`, info.stateDir)
	})

	t.Run("second instance", func(t *testing.T) {
		info, ok := parseInstance("NetBird-private",
			`"C:\Program Files\Netbird\netbird.exe" -s NetBird-private service run --log-level info --daemon-addr npipe://netbird-private`,
			[]string{`NB_STATE_DIR=C:\ProgramData\Netbird-private`, "OTHER=1"})
		assert.True(t, ok)
		assert.Equal(t, "npipe://netbird-private", info.daemonAddr)
		assert.Equal(t, `C:\ProgramData\Netbird-private`, info.stateDir)
	})

	t.Run("not a NetBird service", func(t *testing.T) {
		_, ok := parseInstance("Spooler", `C:\Windows\System32\spoolsv.exe`, nil)
		assert.False(t, ok)
	})

	t.Run("a service command that does not run the daemon", func(t *testing.T) {
		_, ok := parseInstance("x", `netbird.exe service status`, nil)
		assert.False(t, ok)
	})
}
