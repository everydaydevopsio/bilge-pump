//go:build windows

package main

// statfsFreeBytes is not implemented on Windows yet; the free-space delta is
// omitted there.
func statfsFreeBytes(string) (uint64, bool) { return 0, false }
