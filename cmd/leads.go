package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Path to the persistent leads CSV file. We keep it alongside the other data files so
// it survives across program invocations.
var leadsFile = filepath.Join("data", "leads.csv")

// loadLeads returns the slice of raw (un-normalized) addresses stored in the leads file.
// If the file does not exist, an empty slice is returned without error.
func loadLeads() ([]string, error) {
	f, err := os.Open(leadsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no leads yet
		}
		return nil, err
	}
	defer f.Close()

	var addresses []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		addr := strings.TrimSpace(scanner.Text())
		if addr != "" {
			addresses = append(addresses, addr)
		}
	}
	return addresses, scanner.Err()
}

// saveLead appends the given address to the leads file unless it already exists. Comparison
// is done using the same normalization function used for lookups to prevent duplicates with
// differing whitespace/casing.
func saveLead(address string) error {
	normNew := normalize(address)
	existing, err := loadLeads()
	if err != nil {
		return err
	}
	for _, addr := range existing {
		if normalize(addr) == normNew {
			// Already present – nothing to do.
			return nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(leadsFile), 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(leadsFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err = fmt.Fprintln(f, address); err != nil {
		return err
	}
	return nil
}

// showLeads loads the saved leads and presents them in an interactive list similar to
// search results. The user can select a lead to view full property details.
func showLeads(props2025, props2024 map[string]Property) {
	addresses, err := loadLeads()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load leads: %v\n", err)
		return
	}
	if len(addresses) == 0 {
		fmt.Println("No leads saved yet. Use the search mode to add properties to your leads list.")
		return
	}

	var lines []string
	for _, addr := range addresses {
		norm := normalize(addr)
		owner := ""
		if p, ok := props2025[norm]; ok {
			owner = p.OwnerName
		} else if p, ok := props2024[norm]; ok {
			owner = p.OwnerName
		}
		line := fmt.Sprintf("%-40s | %s", addr, owner)
		lines = append(lines, line)
		fmt.Println(line)
	}

	fmt.Println("Use ↑/↓ and Enter for details, Esc to exit.")
	interactiveSelect(addresses, lines, props2025, props2024, false)
}
