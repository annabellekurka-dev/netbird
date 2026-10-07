package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasOtherInterface(t *testing.T) {
	tests := []struct {
		name  string
		ifs   []string
		own   string
		other bool
	}{
		{"none", nil, "wt0", false},
		{"only own", []string{"Ethernet", "wt0"}, "wt0", false},
		{"own case differs", []string{"WT0"}, "wt0", false},
		{"other instance", []string{"Ethernet", "wt0", "wt1"}, "wt0", true},
		{"other instance, own down", []string{"wt1"}, "wt0", true},
		{"unrelated adapters", []string{"Ethernet", "Wi-Fi", "vEthernet (Default Switch)"}, "wt0", false},
		{"prefix only match", []string{"w"}, "wt0", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.other, hasOtherInterface(tc.ifs, tc.own))
		})
	}
}
