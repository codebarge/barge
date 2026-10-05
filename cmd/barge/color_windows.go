//go:build windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

// enableColor turns on ANSI escape processing in the Windows console
// (Windows 10 and newer). Without it, colour codes would show as text.
func enableColor(f *os.File) bool {
	const enableVirtualTerminalProcessing = 0x0004
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getMode := kernel32.NewProc("GetConsoleMode")
	setMode := kernel32.NewProc("SetConsoleMode")
	var mode uint32
	if r, _, _ := getMode.Call(f.Fd(), uintptr(unsafe.Pointer(&mode))); r == 0 {
		return false
	}
	if mode&enableVirtualTerminalProcessing != 0 {
		return true
	}
	r, _, _ := setMode.Call(f.Fd(), uintptr(mode|enableVirtualTerminalProcessing))
	return r != 0
}
