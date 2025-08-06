package main

import (
	"acquisitions/internal/types"
	"bufio"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// ---------------- Large-land remote filter ----------------

// largeLandResult holds the parcel along with parsed acreage and distance from the reference point.
type largeLandResult struct {
	types.Property
	Acres    float64
	Distance float64
}

// findLargeLandFar returns properties that have at least minAcres of land and are located more than
// minMiles away from the provided reference latitude/longitude.
func findLargeLandFar(props []types.Property, minAcres float64, maxAcres float64, refLat, refLon, minMiles float64) []largeLandResult {
	var results []largeLandResult

	for _, p := range props {
		// Parse acreage – ignore blank/unparseable values.
		acresStr := strings.ReplaceAll(strings.TrimSpace(p.LandAcres), ",", "")
		acres, err := strconv.ParseFloat(acresStr, 64)
		if err != nil || acres < minAcres || acres > maxAcres {
			continue
		}

		// Need valid coordinates to compute distance.
		lat, lon, ok := parseLatLon(p.Latitude, p.Longitude)
		if !ok {
			continue
		}

		dist := distanceMiles(refLat, refLon, lat, lon)
		if dist <= minMiles {
			continue
		}

		results = append(results, largeLandResult{
			Property: p,
			Acres:    acres,
			Distance: dist,
		})
	}

	// Order by acreage (desc) then distance (asc) for nicer display.
	sort.Slice(results, func(i, j int) bool {
		if results[i].Acres == results[j].Acres {
			return results[i].Distance < results[j].Distance
		}
		return results[i].Acres > results[j].Acres
	})

	return results
}

// showLargeLandInteractive finds and lists qualifying properties, allowing the user to select one
// for detailed viewing via an interactive list where ←/→ switch pages.
func showLargeLandInteractive() {
	const (
		minAcres         = 10.0
		maxAcres         = 200.0
		refLat   float64 = 32.760089
		refLon   float64 = -97.319828
		minMiles         = 10.0
	)

	// Query database for large land properties
	properties, err := db.QueryLargeLandProperties()
	if err != nil {
		fmt.Printf("Error querying large land properties: %v\n", err)
		return
	}

	results := findLargeLandFar(properties, minAcres, maxAcres, refLat, refLon, minMiles)
	fmt.Printf("\nFound %d properties with >%.0f acres located more than %.0f miles from (%.6f, %.6f)\n", len(results), minAcres, minMiles, refLat, refLon)
	if len(results) == 0 {
		return
	}

	interactiveLargeLand(results)
}

// interactiveLargeLand presents a paginated list (20 per page) of large-land results.
// ↑/↓ navigate within a page, ←/→ change pages, Enter shows details, Esc exits.
func interactiveLargeLand(results []largeLandResult) {
	const pageSize = 20

	if len(results) == 0 {
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
	page := 0
	selected := 0
	totalPages := (len(results) + pageSize - 1) / pageSize

	redraw := func() {
		fmt.Print("\033[H\033[2J")
		start := page * pageSize
		end := start + pageSize
		if end > len(results) {
			end = len(results)
		}
		for i := start; i < end; i++ {
			line := fmt.Sprintf("%-40s | Acres: %5.1f | Dist: %4.1f mi", results[i].SitusAddress, results[i].Acres, results[i].Distance)
			prefix := "  "
			if i-start == selected {
				prefix = "> "
			}
			fmt.Println(prefix + line)
		}
		fmt.Printf("(↑/↓ navigate, ←/→ page, Enter details, Esc quit)  Page %d/%d\n", page+1, totalPages)
	}

	redraw()

	for {
		b1, err := reader.ReadByte()
		if err != nil {
			return
		}

		// Handle Windows console arrow sequences (0 or 224 prefix)
		if b1 == 0 || b1 == 224 {
			b2, _ := reader.ReadByte()
			switch b2 {
			case 72: // up
				if selected > 0 {
					selected--
					redraw()
				}
			case 80: // down
				pageStart := page * pageSize
				pageLen := pageSize
				if pageStart+pageLen > len(results) {
					pageLen = len(results) - pageStart
				}
				if selected < pageLen-1 {
					selected++
					redraw()
				}
			case 75: // left
				if page > 0 {
					page--
					selected = 0
					redraw()
				}
			case 77: // right
				if page < totalPages-1 {
					page++
					selected = 0
					redraw()
				}
			case 13: // Enter (handled later as well)
			}
			continue
		}

		switch b1 {
		case 27: // ESC or ANSI sequence
			if reader.Buffered() == 0 {
				fmt.Println()
				return
			}
			b2, _ := reader.ReadByte()
			if b2 != '[' {
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
				pageStart := page * pageSize
				pageLen := pageSize
				if pageStart+pageLen > len(results) {
					pageLen = len(results) - pageStart
				}
				if selected < pageLen-1 {
					selected++
					redraw()
				}
			case 'D': // left
				if page > 0 {
					page--
					selected = 0
					redraw()
				}
			case 'C': // right
				if page < totalPages-1 {
					page++
					selected = 0
					redraw()
				}
			}
		case '\r', '\n': // Enter
			idx := page*pageSize + selected
			if idx < len(results) {
				term.Restore(fd, oldState)
				fmt.Println()
				lookupAndRender(results[idx].SitusAddress, true)

				fmt.Print("\n(press Enter to return)")
				_, _ = bufio.NewReader(os.Stdin).ReadBytes('\n')

				oldState, err = term.MakeRaw(fd)
				if err != nil {
					return
				}
				if runtime.GOOS == "windows" {
					enableVT()
				}
				reader = bufio.NewReader(os.Stdin)
				redraw()
			}
		case 3: // Ctrl-C
			fmt.Println()
			return
		default:
			// ignore other keys
		}
	}
}
