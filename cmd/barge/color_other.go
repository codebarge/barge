//go:build !windows

package main

import "os"

// enableColor reports whether f can show colours; Unix terminals always can.
func enableColor(*os.File) bool { return true }
