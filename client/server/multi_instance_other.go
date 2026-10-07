//go:build !windows

package server

// otherInstanceActive is always false off Windows: running several instances
// side by side is only supported there.
func otherInstanceActive(string) bool {
	return false
}
