package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDebugTriggerEventNameFor(t *testing.T) {
	assert.Equal(t, `Global\NetbirdDebugTriggerEvent`, debugTriggerEventNameFor(""))
	assert.Equal(t, `Global\NetbirdDebugTriggerEvent`, debugTriggerEventNameFor("wt0"))
	assert.Equal(t, `Global\NetbirdDebugTriggerEvent`, debugTriggerEventNameFor("WT0"))
	assert.Equal(t, `Global\NetbirdDebugTriggerEvent_wt1`, debugTriggerEventNameFor("wt1"))
	assert.Equal(t, `Global\NetbirdDebugTriggerEvent_wt1`, debugTriggerEventNameFor("WT1"))
}
