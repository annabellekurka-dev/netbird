//go:build windows

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQuickActionsEventName(t *testing.T) {
	assert.Equal(t, `Global\NetBirdQuickActionsTriggerEvent`, quickActionsEventName(""))
	assert.Equal(t, `Global\NetBirdQuickActionsTriggerEvent_netbird-private`, quickActionsEventName("netbird-private"))
}
