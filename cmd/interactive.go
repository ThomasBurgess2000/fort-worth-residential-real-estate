package main

import (
	"bufio"
	"fmt"
	"os"
	"runtime"

	"golang.org/x/term"
)

// interactiveSelect lets user move through the provided lines with arrow keys and press Enter to
// view full property details. It expects len(addresses)==len(lines).
func interactiveSelect(addresses []string, lines []string, askSave bool) {
	if len(addresses) == 0 {
		return
	}

	if runtime.GOOS == "windows" {
		enableVT()
	}

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		fmt.Println("(interactive selection not supported on this terminal)")
		return
	}
	defer term.Restore(fd, oldState)

	reader := bufio.NewReader(os.Stdin)

	selected := 0

	redraw := func() {
		// Clear screen (ANSI reset to top + clear screen)
		fmt.Print("\033[H\033[2J")
		for i, l := range lines {
			prefix := "  "
			if i == selected {
				prefix = "> "
			}
			fmt.Println(prefix + l)
		}
		fmt.Println("(↑/↓ to navigate, Enter to view details, Esc to quit)")
	}

	redraw()

	for {
		b1, err := reader.ReadByte()
		if err != nil {
			return
		}
		// Handle Windows console arrow sequences (0 or 224, then code)
		if b1 == 0 || b1 == 224 {
			b2, _ := reader.ReadByte()
			switch b2 {
			case 72: // up
				if selected > 0 {
					selected--
					redraw()
				}
			case 80: // down
				if selected < len(addresses)-1 {
					selected++
					redraw()
				}
			case 13: // Enter
				term.Restore(fd, oldState)
				fmt.Println()
				lookupAndRender(addresses[selected], askSave)

				// Wait for user acknowledgement before returning to list
				fmt.Print("\n(press Enter to return)")
				_, _ = bufio.NewReader(os.Stdin).ReadBytes('\n')

				oldState, err = term.MakeRaw(fd)
				if err != nil {
					return
				}
				reader = bufio.NewReader(os.Stdin)
				redraw()
			}
			continue
		}

		switch b1 {
		case 27: // ESC or ANSI sequence
			if reader.Buffered() == 0 {
				// Bare ESC – exit
				fmt.Println()
				return
			}
			b2, _ := reader.ReadByte()
			if b2 != '[' {
				// Not a CSI sequence; ignore unknown combo
				continue
			}
			if reader.Buffered() == 0 {
				continue
			}
			b3, _ := reader.ReadByte()
			switch b3 {
			case 'A': // up
				if selected > 0 {
					selected--
					redraw()
				}
			case 'B': // down
				if selected < len(addresses)-1 {
					selected++
					redraw()
				}
			}
		case '\r', '\n': // Enter
			term.Restore(fd, oldState) // restore cooked mode before rendering details
			fmt.Println()
			lookupAndRender(addresses[selected], askSave)

			// Wait for user acknowledgement before returning to list
			fmt.Print("\n(press Enter to return)")
			_, _ = bufio.NewReader(os.Stdin).ReadBytes('\n')

			// After displaying details, re-enter raw mode for potential further navigation.
			oldState, err = term.MakeRaw(fd)
			if err != nil {
				return
			}
			reader = bufio.NewReader(os.Stdin)
			redraw()
		case 3: // Ctrl-C
			fmt.Println()
			return

		default:
			// ignore other keys
		}
	}
}
