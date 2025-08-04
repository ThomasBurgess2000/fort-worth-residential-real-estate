package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Path to the Obsidian kanban board that stores lead addresses and the directory
// that holds individual lead markdown files. These are resolved relative to the
// user profile so the program works regardless of the exact Windows username.
var (
	leadsBoardFile  = filepath.Join(os.Getenv("USERPROFILE"), "Desktop", "Acquisitions", "Leads.md")
	leadsDetailsDir = filepath.Join(os.Getenv("USERPROFILE"), "Desktop", "Acquisitions", "Leads")
)

// loadLeads returns the slice of raw (un-normalized) addresses stored in the
// **Unscreened** section of the kanban board. If the file does not exist, an
// empty slice is returned without error so the rest of the program can operate
// unaffected.
func loadLeads() ([]string, error) {
	f, err := os.Open(leadsBoardFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no leads yet
		}
		return nil, err
	}
	defer f.Close()

	var addresses []string
	scanner := bufio.NewScanner(f)
	inUnscreened := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Detect headers. Any new header ends the Unscreened section.
		if strings.HasPrefix(line, "## ") {
			if strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(line, "## ")), "Unscreened") {
				inUnscreened = true
				continue
			}
			if inUnscreened {
				break // finished with Unscreened section
			}
		}
		if !inUnscreened {
			continue
		}
		if strings.HasPrefix(line, "-") {
			addr := extractAddressFromBullet(line)
			if addr != "" {
				addresses = append(addresses, addr)
			}
		}
	}
	return addresses, scanner.Err()
}

// saveLead appends the property address to the Unscreened list (without adding
// duplicates) and creates a markdown file for the property using the fields
// available in the Property struct.
func saveLead(prop Property) error {
	address := strings.TrimSpace(prop.SitusAddress)
	if address == "" {
		return fmt.Errorf("property has empty address – cannot save lead")
	}

	// First make sure we are not introducing a duplicate.
	existing, err := loadLeads()
	if err != nil {
		return err
	}
	normNew := normalize(address)
	for _, addr := range existing {
		if normalize(addr) == normNew {
			// Already present – nothing else to do except ensure file exists.
			return createLeadDetailFile(prop)
		}
	}

	// Ensure board directory exists.
	if err := os.MkdirAll(filepath.Dir(leadsBoardFile), 0755); err != nil {
		return err
	}

	// Read the entire file (if any) so we can insert into the Unscreened section.
	var content []byte
	if b, err := os.ReadFile(leadsBoardFile); err == nil {
		content = b
	}

	// Build bullet to insert.
	bullet := fmt.Sprintf("- [ ] [[%s]]", address)

	var out bytes.Buffer
	if len(content) == 0 {
		// Fresh board – create minimal structure.
		out.WriteString("## Unscreened\n\n")
		out.WriteString(bullet + "\n\n")
	} else {
		lines := strings.Split(string(content), "\n")
		inserted := false
		inUnscreened := false
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "## ") {
				if strings.EqualFold(strings.TrimPrefix(trimmed, "## "), "Unscreened") {
					inUnscreened = true
					// Write the header line back.
					out.WriteString(line + "\n")
					continue
				}
				if inUnscreened && !inserted {
					// We reached the next header without inserting – add bullet before it.
					out.WriteString(bullet + "\n")
					inserted = true
					inUnscreened = false // no longer in unscreened section
				}
			}
			if inUnscreened && i == len(lines)-1 && !inserted {
				// Unscreened is last section. Append at end.
				out.WriteString(line + "\n")
				out.WriteString(bullet + "\n")
				inserted = true
				continue
			}
			out.WriteString(line + "\n")
		}
		if !inserted && !inUnscreened {
			// No Unscreened header at all – append one.
			out.WriteString("\n## Unscreened\n\n" + bullet + "\n")
		}
	}

	if err := os.WriteFile(leadsBoardFile, out.Bytes(), 0644); err != nil {
		return err
	}

	// Finally create individual lead file (if not already present).
	return createLeadDetailFile(prop)
}

// createLeadDetailFile writes the detailed markdown file for the property unless it already exists.
func createLeadDetailFile(prop Property) error {
	if err := os.MkdirAll(leadsDetailsDir, 0755); err != nil {
		return err
	}
	filename := sanitizeFileName(prop.SitusAddress) + ".md"
	path := filepath.Join(leadsDetailsDir, filename)
	if _, err := os.Stat(path); err == nil {
		return nil // already exists – leave it untouched
	}

	var b bytes.Buffer
	fmt.Fprintln(&b, "## Location Info")
	fmt.Fprintln(&b, "- Zip Code: ")
	fmt.Fprintf(&b, "- Subdivision: %s\n", prop.Subdivision)

	fmt.Fprintln(&b, "## Owner Info")
	fmt.Fprintf(&b, "- Owner Name: %s\n", prop.OwnerName)
	ownerAddr := buildOwnerAddress(prop)
	fmt.Fprintf(&b, "- Owner Address: %s\n", ownerAddr)
	fmt.Fprintln(&b, "- Phone: ")
	fmt.Fprintln(&b, "- Email: ")
	fmt.Fprintf(&b, "- Last Sale Date: %s\n", prop.LastSaleDate)

	fmt.Fprintln(&b, "## Property Info")
	fmt.Fprintf(&b, "- Total Value: %s\n", prop.TotalValue)
	fmt.Fprintf(&b, "\t- Improvement: %s\n", prop.ImprovementValue)
	fmt.Fprintf(&b, "\t- Land: %s\n", prop.LandValue)
	fmt.Fprintf(&b, "- Year Built: %s\n", prop.YearBuilt)
	landLine := strings.TrimSpace(fmt.Sprintf("%s acres / %s sqft", prop.LandAcres, prop.LandSqFt))
	fmt.Fprintf(&b, "- Land: %s\n", landLine)
	fmt.Fprintf(&b, "- Living Area (sf): %s\n", prop.LivingArea)
	bedsBaths := strings.TrimSpace(fmt.Sprintf("%s/%s", prop.NumBedrooms, prop.NumBathrooms))
	fmt.Fprintf(&b, "- Bedrooms/Bath: %s\n", bedsBaths)
	// Determine zoning via shapefile lookup (same logic as property renderer)
	zoningCode := ""
	if latDeg, lonDeg, ok := parseLatLon(prop.Latitude, prop.Longitude); ok && len(zoningFeatures) > 0 {
		latFt, lonFt := wgs84ToTxNC(latDeg, lonDeg)
		if attrs, found := findZoningAttributes(latFt, lonFt); found {
			if z, ok := attrs["ZONING"]; ok && strings.TrimSpace(z) != "" {
				zoningCode = strings.TrimSpace(z)
			} else if z, ok := attrs["BASE_ZONIN"]; ok && strings.TrimSpace(z) != "" {
				zoningCode = strings.TrimSpace(z)
			}
		}
	}
	fmt.Fprintf(&b, "- Zoning: %s\n", zoningCode)
	fmt.Fprintf(&b, "- Site Class: %s\n", prop.SiteClassDescr)
	fmt.Fprintf(&b, "- TAD URL: https://www.tad.org/property?account=%s\n", prop.AccountNum)

	fmt.Fprintln(&b, "## Notes:")

	return os.WriteFile(path, b.Bytes(), fs.FileMode(0644))
}

// extractAddressFromBullet attempts to pull the address out of a markdown bullet
// of the form "- [ ] [[ADDRESS]]" (preferred) or "- [ ] ADDRESS".
func extractAddressFromBullet(line string) string {
	// Strip leading "-" and checkbox markup.
	line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
	if strings.HasPrefix(line, "[ ]") || strings.HasPrefix(line, "[x]") || strings.HasPrefix(line, "[X]") {
		line = strings.TrimSpace(line[3:])
	}
	// Wiki-link style [[ADDRESS]].
	if strings.HasPrefix(line, "[[") && strings.HasSuffix(line, "]]") {
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "[["), "]]"))
	}
	return strings.TrimSpace(line)
}

// sanitizeFileName replaces characters that are illegal in Windows file names so
// we can safely create a markdown file for the address.
func sanitizeFileName(name string) string {
	invalid := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	for _, c := range invalid {
		name = strings.ReplaceAll(name, c, "_")
	}
	return name
}

// buildOwnerAddress concatenates the owner address fields, gracefully handling
// missing pieces so we don't end up with awkward stray commas or spaces.
func buildOwnerAddress(p Property) string {
	parts := []string{}
	if p.OwnerAddress != "" {
		parts = append(parts, strings.TrimSpace(p.OwnerAddress))
	}
	if p.OwnerCityState != "" {
		parts = append(parts, strings.TrimSpace(p.OwnerCityState))
	}
	if p.OwnerZip != "" {
		parts = append(parts, strings.TrimSpace(p.OwnerZip))
	}
	return strings.Join(parts, ", ")
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
