package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"acquisitions/internal/database"
	"acquisitions/internal/types"
)

// Global database instance
var db *database.Database

const (
	colorRed   = "\033[31m"
	colorGreen = "\033[32m"
	colorReset = "\033[0m"
)

func main() {
	_ = time.Now()

	// Load zoning polygons first so they're available for lookups.
	if err := initZoning(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}

	// Initialize database connection
	dbConfig := database.LoadDatabaseConfig()
	var err error
	db, err = database.NewDatabase(dbConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	fmt.Println("Connected to Oracle Autonomous Database")

	// If the user provided an argument on the command line, decide whether it's a zip or an address.
	if len(os.Args) > 1 {
		arg := os.Args[1]
		// Special command: list large rural parcels (>10 acres & >10mi from downtown)
		if strings.EqualFold(arg, "bigland") {
			showLargeLandInteractive()
			return
		}
		if strings.HasPrefix(arg, "sub=") || strings.HasPrefix(arg, "sub:") {
			sub := strings.TrimPrefix(strings.TrimPrefix(arg, "sub="), "sub:")
			handleSubdivisionQuery(sub)
			return
		}
		// Otherwise treat the argument(s) as an address lookup.
		address := strings.Join(os.Args[1:], " ")
		lookupAndRender(address, true)
		return
	}

	// Interactive loop for multiple lookups.
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("Enter address, sub=<Subdivision>, 'leads', or 'bigland' (blank to quit): ")
		input, _ := reader.ReadString('\n')
		addrInput := strings.TrimSpace(input)
		if addrInput == "" {
			break
		}
		// Special command: show leads list
		if strings.EqualFold(addrInput, "leads") {
			showLeads()
			continue
		}
		// Special command: list large rural parcels (>10 acres & >10mi from downtown)
		if strings.EqualFold(addrInput, "bigland") {
			showLargeLandInteractive()
			continue
		}

		// Subdivision query
		if strings.HasPrefix(addrInput, "sub=") || strings.HasPrefix(addrInput, "sub:") {
			sub := strings.TrimPrefix(strings.TrimPrefix(addrInput, "sub="), "sub:")
			handleSubdivisionQuery(sub)
			continue
		}

		// Default: treat input as an address search
		lookupAndRender(addrInput, true)
	}
}

// lookupAndRender searches the database for the given address and displays the result.
func lookupAndRender(address string, askSave bool) {
	norm := normalize(address)

	// Query 2025 data first
	prop2025, err := db.QueryPropertyByAddress(norm)
	if err != nil {
		fmt.Printf("Error querying 2025 data: %v\n", err)
		return
	}

	// Query 2024 data
	prop2024, err := db.QueryPropertyByAddress2024(norm)
	if err != nil {
		fmt.Printf("Error querying 2024 data: %v\n", err)
		return
	}

	// selProp points to the Property we ultimately displayed (2025 preferred, else 2024).
	var selProp *types.Property

	if prop2025 != nil {
		selProp = prop2025
		if prop2024 != nil {
			renderPropertyDiff(*prop2025, *prop2024)
		} else {
			renderPropertyDiff(*prop2025, types.Property{})
		}
	} else if prop2024 != nil {
		selProp = prop2024
		fmt.Println("[Note] No 2025 record found; displaying 2024 data")
		renderPropertyDiff(*prop2024, types.Property{})
	} else {
		fmt.Printf("No property found for address: %s\n", address)
		return
	}

	if askSave {
		// Offer to save the property as a lead.
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Save to leads? (y/N): ")
		resp, _ := reader.ReadString('\n')
		resp = strings.ToLower(strings.TrimSpace(resp))
		if resp == "y" || resp == "yes" {
			if err := saveLead(*selProp); err != nil {
				fmt.Printf("Failed to save lead: %v\n", err)
			} else {
				fmt.Println("Lead saved.")
			}
		}
	}
}

// normalize produces a canonical form of an address key.
func normalize(addr string) string {
	addr = strings.ToUpper(strings.TrimSpace(addr))
	addr = strings.ReplaceAll(addr, ",", "")
	addr = strings.Join(strings.Fields(addr), " ") // collapse whitespace
	return addr
}

// renderProperty prints the property information in a pleasant, readable layout.
func renderPropertyDiff(cur types.Property, prev types.Property) {
	diff := func(a, b string) string {
		if b != "" && a != b {
			return fmt.Sprintf(" %s[%s]%s", colorRed, b, colorReset)
		}
		return ""
	}

	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("Address           : %s\n", cur.SitusAddress)
	fmt.Printf("Subdivision       : %s%s\n", cur.Subdivision, diff(cur.Subdivision, prev.Subdivision))

	fmt.Printf("Owner             : %s%s\n", cur.OwnerName, diff(cur.OwnerName, prev.OwnerName))
	curAddr := fmt.Sprintf("%s, %s %s", cur.OwnerAddress, cur.OwnerCityState, cur.OwnerZip)
	prevAddr := fmt.Sprintf("%s, %s %s", prev.OwnerAddress, prev.OwnerCityState, prev.OwnerZip)

	norm := func(s string) string { return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), ",", "")) }

	sameTag := ""
	if norm(cur.OwnerAddress) == norm(cur.SitusAddress) {
		sameTag = fmt.Sprintf(" %s[Same]%s", colorGreen, colorReset)
	}

	diffTag := diff(curAddr, prevAddr)
	fmt.Printf("Owner Address     : %s%s%s\n", curAddr, sameTag, diffTag)
	fmt.Printf("Last Sale Date    : %s%s\n", cur.LastSaleDate, diff(cur.LastSaleDate, prev.LastSaleDate))
	fmt.Println()

	fmt.Printf("Condition         : %s%s\n", cur.Condition, diff(cur.Condition, prev.Condition))
	fmt.Printf("Quality           : %s%s\n", cur.Quality, diff(cur.Quality, prev.Quality))
	fmt.Printf("Depreciation %%    : %s%s\n", cur.DepreciationPercent, diff(cur.DepreciationPercent, prev.DepreciationPercent))
	fmt.Println()

	fmt.Printf("Total Value       : %s%s\n", cur.TotalValue, diff(cur.TotalValue, prev.TotalValue))
	fmt.Printf("  Improvement     : %s%s\n", cur.ImprovementValue, diff(cur.ImprovementValue, prev.ImprovementValue))
	fmt.Printf("  Land            : %s%s\n", cur.LandValue, diff(cur.LandValue, prev.LandValue))
	fmt.Printf("Year Built        : %s%s\n", cur.YearBuilt, diff(cur.YearBuilt, prev.YearBuilt))
	fmt.Println()

	fmt.Printf("Land              : %s acres / %s sqft%s\n", cur.LandAcres, cur.LandSqFt, diff(cur.LandAcres+"/"+cur.LandSqFt, prev.LandAcres+"/"+prev.LandSqFt))
	fmt.Printf("Living Area (sf)  : %s%s\n", cur.LivingArea, diff(cur.LivingArea, prev.LivingArea))
	fmt.Printf("Bedrooms/Bath     : %s / %s%s\n", cur.NumBedrooms, cur.NumBathrooms, diff(cur.NumBedrooms+"/"+cur.NumBathrooms, prev.NumBedrooms+"/"+prev.NumBathrooms))
	fmt.Println()

	fmt.Printf("Site Class        : %s%s\n", cur.SiteClassDescr, diff(cur.SiteClassDescr, prev.SiteClassDescr))
	fmt.Printf("TAD URL           : https://www.tad.org/property?account=%s\n", cur.AccountNum)

	// Zoning lookup via shapefile
	latDeg, lonDeg, ok := parseLatLon(cur.Latitude, cur.Longitude)
	if ok && len(zoningFeatures) > 0 {
		latFt, lonFt := wgs84ToTxNC(latDeg, lonDeg)
		if attrs, found := findZoningAttributes(latFt, lonFt); found {
			if z, ok := attrs["ZONING"]; ok && strings.TrimSpace(z) != "" {
				fmt.Printf("Zoning            : %s\n", strings.TrimSpace(z))
			} else if z, ok := attrs["BASE_ZONIN"]; ok && strings.TrimSpace(z) != "" {
				fmt.Printf("Zoning            : %s\n", strings.TrimSpace(z))
			} else {
				fmt.Println("Zoning attributes found but zoning code missing")
			}
		} else {
			fmt.Println("No zoning attributes found")
		}
	} else {
		fmt.Println("Latitude/Longitude unavailable; cannot determine zoning")
	}
	fmt.Println(strings.Repeat("-", 80))
}

// ---------------- Subdivision undervaluation analysis ----------------

type undervaluedResult struct {
	types.Property
	NeighborCount int
	Mean          float64
	StdDev        float64
}

// findUndervaluedInSubdivision returns properties in the given subdivision whose ImprovementValue
// is at least one standard deviation below neighboring comps (0.25 mi).
func findUndervaluedInSubdivision(sub string, props []types.Property) []undervaluedResult {
	sub = strings.ToUpper(strings.TrimSpace(sub))
	// Collect candidates in subdivision with coords & value.
	var candidates []types.Property
	for _, p := range props {
		if strings.ToUpper(strings.TrimSpace(p.Subdivision)) == sub && p.Latitude != "" && p.Longitude != "" && p.ImprovementValue != "" {
			candidates = append(candidates, p)
		}
	}
	return undervaluedFromCandidates(candidates, props)
}

// undervaluedFromCandidates runs the spatial+stat comparison for a set of candidate
// properties and returns those that are at least one standard deviation under the mean.
func undervaluedFromCandidates(candidates []types.Property, universe []types.Property) []undervaluedResult {
	var results []undervaluedResult
	for _, p := range candidates {
		lat1, lon1, ok := parseLatLon(p.Latitude, p.Longitude)
		if !ok {
			continue
		}
		val, ok := parseDollar(p.ImprovementValue)
		if !ok {
			continue
		}

		var neighborVals []float64
		for _, q := range universe {
			if q.Latitude == "" || q.Longitude == "" || q.ImprovementValue == "" {
				continue
			}
			lat2, lon2, ok := parseLatLon(q.Latitude, q.Longitude)
			if !ok {
				continue
			}
			if distanceMiles(lat1, lon1, lat2, lon2) <= 0.1 {
				if v, ok := parseDollar(q.ImprovementValue); ok {
					neighborVals = append(neighborVals, v)
				}
			}
		}

		if len(neighborVals) < 3 { // need a few comps to be meaningful
			continue
		}
		mean, std := meanStd(neighborVals)
		if val < mean-std {
			results = append(results, undervaluedResult{
				Property:      p,
				NeighborCount: len(neighborVals),
				Mean:          mean,
				StdDev:        std,
			})
		}
	}
	return results
}

func parseLatLon(latStr, lonStr string) (float64, float64, bool) {
	lat, err1 := strconv.ParseFloat(strings.TrimSpace(latStr), 64)
	lon, err2 := strconv.ParseFloat(strings.TrimSpace(lonStr), 64)
	return lat, lon, err1 == nil && err2 == nil
}

func parseDollar(s string) (float64, bool) {
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

func distanceMiles(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusMiles = 3958.8
	toRad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusMiles * c
}

func meanStd(vals []float64) (mean, std float64) {
	for _, v := range vals {
		mean += v
	}
	mean /= float64(len(vals))
	for _, v := range vals {
		std += (v - mean) * (v - mean)
	}
	std = math.Sqrt(std / float64(len(vals)))
	return
}
