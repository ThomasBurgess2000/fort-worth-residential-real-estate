//go:build windows
// +build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// enableVT enables virtual terminal input/output so that ANSI escape sequences
// are delivered to the program and interpreted by the console.
func enableVT() {
	// Input side
	hIn := windows.Handle(os.Stdin.Fd())
	var inMode uint32
	if windows.GetConsoleMode(hIn, &inMode) == nil {
		windows.SetConsoleMode(hIn, inMode|windows.ENABLE_VIRTUAL_TERMINAL_INPUT)
	}

	// Output side
	hOut := windows.Handle(os.Stdout.Fd())
	var outMode uint32
	if windows.GetConsoleMode(hOut, &outMode) == nil {
		windows.SetConsoleMode(hOut, outMode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	}
}
