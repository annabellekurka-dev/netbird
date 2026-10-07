package updater

// isAutoUpdateSupported is false in this fork: an update would replace the
// patched binaries with the official ones.
func isAutoUpdateSupported() bool {
	return false
}
