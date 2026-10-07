package device

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSafeAliasPattern(t *testing.T) {
	for _, ok := range []string{"wt1", "wt10", "wt_private", "WT-2"} {
		assert.True(t, safeAliasPattern.MatchString(ok), ok)
	}
	for _, bad := range []string{"", "wt1'; calc; '", "wt 1", "wt1\n", "a-very-long-interface-name", `wt1"`, "wt$x"} {
		assert.False(t, safeAliasPattern.MatchString(bad), bad)
	}
}

func TestNetworkCategoryScript(t *testing.T) {
	script := networkCategoryScript("wt1")

	assert.Contains(t, script, "Get-NetConnectionProfile -InterfaceAlias 'wt1'")
	assert.Contains(t, script, "Set-NetConnectionProfile -InterfaceAlias 'wt1' -NetworkCategory Private")
	assert.Contains(t, script, "-ne 'Private'", "an interface already on Private is left alone")
}

func TestIsClaimedFollowsClaim(t *testing.T) {
	name := "wtclaimtest3"
	t.Cleanup(func() { releaseInterface(name) })

	assert.False(t, isClaimed(name))
	assert.NoError(t, claimInterface(name))
	assert.True(t, isClaimed(name))
	releaseInterface(name)
	assert.False(t, isClaimed(name), "the retry loop stops once the interface is closed")
}
