package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestServiceDisplayName(t *testing.T) {
	assert.Equal(t, "Netbird", serviceDisplayName("Netbird"))
	assert.Equal(t, "Netbird", serviceDisplayName("netbird"))
	assert.Equal(t, "Netbird (NetBird-private)", serviceDisplayName("NetBird-private"))
}
