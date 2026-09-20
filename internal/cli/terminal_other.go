//go:build !linux && !darwin && !windows

package cli

// Platforms without terminal detection must use explicit --yes confirmation.
func isTerminal(fd uintptr) bool { return false }
