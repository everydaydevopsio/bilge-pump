//go:build !windows

package main

import "syscall"

// statfsFreeBytes returns the bytes available to unprivileged users on the
// filesystem holding path.
func statfsFreeBytes(path string) (uint64, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, false
	}
	return uint64(st.Bavail) * uint64(st.Bsize), true
}
