package dns

import (
	"fmt"
)

type ShutdownState struct {
	Guid string
	GPO  bool
	// KeyPrefix is empty for the default instance, which keeps the state file
	// of a default install unchanged.
	KeyPrefix string `json:",omitempty"`
}

func (s *ShutdownState) Name() string {
	return "dns_state"
}

func (s *ShutdownState) Cleanup() error {
	manager := &registryConfigurator{
		guid:      s.Guid,
		gpo:       s.GPO,
		keyPrefix: s.KeyPrefix,
	}

	if err := manager.restoreUncleanShutdownDNS(); err != nil {
		return fmt.Errorf("restore unclean shutdown dns: %w", err)
	}

	return nil
}
