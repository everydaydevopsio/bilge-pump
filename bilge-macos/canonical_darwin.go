//go:build darwin

package main

import (
	"bytes"
	"os"
	"syscall"
	"unsafe"
)

// kernelPath asks the kernel for an open file's path (fcntl F_GETPATH). It
// resolves symlinks and ".." the way the kernel does, and returns the on-disk
// spelling, so case aliases on case-insensitive APFS volumes match.
func kernelPath(p string) (string, bool) {
	f, err := os.OpenFile(p, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", false
	}
	defer func() { _ = f.Close() }()
	var buf [1024]byte // MAXPATHLEN on macOS
	if _, _, errno := syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), syscall.F_GETPATH, uintptr(unsafe.Pointer(&buf[0]))); errno != 0 {
		return "", false
	}
	n := bytes.IndexByte(buf[:], 0)
	if n <= 0 {
		return "", false
	}
	return string(buf[:n]), true
}
