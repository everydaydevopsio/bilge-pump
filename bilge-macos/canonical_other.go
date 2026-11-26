//go:build !darwin

package main

// kernelPath is only available on macOS; other platforms use EvalSymlinks.
func kernelPath(string) (string, bool) { return "", false }
