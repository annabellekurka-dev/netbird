//go:build !windows && !ios && !android

package cmd

// installedInstances lists nothing off Windows: the instance guard only runs there.
func installedInstances() []instanceInfo {
	return nil
}
