package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Property holds combined data from the primary and supplemental datasets.
// We keep only a subset of interesting fields; add more as needed.
type Property struct {
	AccountNum     string
	SitusAddress   string
	OwnerName      string
	OwnerAddress   string
	OwnerCityState string
	OwnerZip       string
	Subdivision    string

	LastSaleDate        string
	Condition           string
	DepreciationPercent string
	Quality             string

	TotalValue       string
	ImprovementValue string
	LandValue        string

	YearBuilt    string
	LivingArea   string
	NumBedrooms  string
	NumBathrooms string

	SiteClassDescr string
	PropertyClass  string
	StateUseCode   string
	LandAcres      string
	LandSqFt       string
	SiteClassCd    string
	LandUseCode    string

	County         string
	City           string
	SchoolDistrict string

	DeedDate     string
	ARBIndicator string

	Latitude  string
	Longitude string
}

// Dataset paths. Adjust if your directory layout changes.
var (
	primaryFile      = filepath.Join("data", "PropertyData_R_2025.txt")
	supplementalFile = filepath.Join("data", "PropertyDataSupplemental_R_2025.txt")
	primary2024File  = filepath.Join("data", "PropertyData_2024.txt")
)

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
	datasetStart := time.Now()

	// Load datasets
	props2025, props2024, err := loadDatasets()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load datasets: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Datasets loaded in %v (%d records)\n", time.Since(datasetStart).Truncate(time.Millisecond), len(props2025))

	// If the user provided an argument on the command line, decide whether it's a zip or an address.
	if len(os.Args) > 1 {
		arg := os.Args[1]
		if strings.HasPrefix(arg, "sub=") || strings.HasPrefix(arg, "sub:") {
			sub := strings.TrimPrefix(strings.TrimPrefix(arg, "sub="), "sub:")
			handleSubdivisionQuery(sub, props2025, props2024)
			return
		}
		// Otherwise treat the argument(s) as an address lookup.
		address := strings.Join(os.Args[1:], " ")
		lookupAndRender(address, props2025, props2024, true)
		return
	}

	// Interactive loop for multiple lookups.
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("Enter address, sub=<Subdivision>, or 'leads' (blank to quit): ")
		input, _ := reader.ReadString('\n')
		addrInput := strings.TrimSpace(input)
		if addrInput == "" {
			break
		}
		// Special command: show leads list
		if strings.EqualFold(addrInput, "leads") {
			showLeads(props2025, props2024)
			continue
		}

		// Subdivision query
		if strings.HasPrefix(addrInput, "sub=") || strings.HasPrefix(addrInput, "sub:") {
			sub := strings.TrimPrefix(strings.TrimPrefix(addrInput, "sub="), "sub:")
			handleSubdivisionQuery(sub, props2025, props2024)
			continue
		}

		// Default: treat input as an address search
		lookupAndRender(addrInput, props2025, props2024, true)
	}
}

// lookupAndRender searches the 2025 and 2024 maps for the given address and displays the result.
func lookupAndRender(address string, props2025 map[string]Property, props2024 map[string]Property, askSave bool) {
	norm := normalize(address)
	prop2025, ok2025 := props2025[norm]
	prop2024, ok2024 := props2024[norm]

	// selProp points to the Property we ultimately displayed (2025 preferred, else 2024).
	var selProp *Property

	if ok2025 {
		selProp = &prop2025
		if ok2024 {
			renderPropertyDiff(prop2025, prop2024)
		} else {
			renderPropertyDiff(prop2025, Property{})
		}
	} else if ok2024 {
		selProp = &prop2024
		fmt.Println("[Note] No 2025 record found; displaying 2024 data")
		renderPropertyDiff(prop2024, Property{})
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

// loadDatasets reads both data files, merges them by Account Number, and returns a map keyed by normalized address.
func loadDatasets() (map[string]Property, map[string]Property, error) {
	// First read primary file into map keyed by account number.
	primaryByAcct := make(map[string]Property)
	var muPrimary sync.Mutex
	if err := readFile(primaryFile, func(record map[string]string) {
		acct := record["Account_Num"]
		prop := Property{
			AccountNum:       acct,
			SitusAddress:     record["Situs_Address"],
			OwnerName:        record["Owner_Name"],
			OwnerAddress:     record["Owner_Address"],
			OwnerCityState:   record["Owner_CityState"],
			OwnerZip:         record["Owner_Zip"],
			Subdivision:      record["SubdivisionName"],
			County:           record["County"],
			City:             record["City"],
			SchoolDistrict:   record["School"],
			LandValue:        record["Land_Value"],
			ImprovementValue: record["Improvement_Value"],
			TotalValue:       record["Total_Value"],

			DeedDate:     record["Deed_Date"],
			ARBIndicator: record["ARB_Indicator"],

			YearBuilt:    record["Year_Built"],
			LivingArea:   record["Living_Area"],
			NumBedrooms:  record["Num_Bedrooms"],
			NumBathrooms: record["Num_Bathrooms"],

			PropertyClass: record["Property_Class"],
			StateUseCode:  record["State_Use_Code"],

			LandAcres: record["Land_Acres"],
			LandSqFt:  record["Land_SqFt"],
		}
		muPrimary.Lock()
		primaryByAcct[acct] = prop
		muPrimary.Unlock()
	}); err != nil {
		return nil, nil, err
	}

	// Now read supplemental file and merge.
	if err := readFile(supplementalFile, func(record map[string]string) {
		acct := record["AccountNumber"]
		muPrimary.Lock()
		prop, ok := primaryByAcct[acct]
		if !ok {
			muPrimary.Unlock()
			// Only merge if we already have the base record.
			return
		}
		prop.Latitude = record["Latitude"]
		prop.Longitude = record["Longitude"]
		prop.Quality = record["Quality"]

		prop.LastSaleDate = record["LastSaleDate"]
		prop.Condition = record["Condition"]
		prop.DepreciationPercent = record["DepreciationPercent"]

		prop.Subdivision = record["SubdivisionName"]
		prop.SiteClassCd = record["SiteClassCd"]
		prop.SiteClassDescr = record["SiteClassDescr"]
		prop.LandUseCode = record["LandUseCode"]
		primaryByAcct[acct] = prop
		muPrimary.Unlock()
	}); err != nil {
		return nil, nil, err
	}

	// Build address map.
	byAddress := make(map[string]Property, len(primaryByAcct))
	for _, prop := range primaryByAcct {
		addrNorm := normalize(prop.SitusAddress)
		byAddress[addrNorm] = prop
	}
	// Now load 2024 primary dataset and build address map.
	primary2024ByAcct := make(map[string]Property)
	var mu2024 sync.Mutex
	if err := readFile(primary2024File, func(record map[string]string) {
		acct := record["Account_Num"]
		prop := Property{
			AccountNum:       acct,
			SitusAddress:     record["Situs_Address"],
			OwnerName:        record["Owner_Name"],
			OwnerAddress:     record["Owner_Address"],
			OwnerCityState:   record["Owner_CityState"],
			OwnerZip:         record["Owner_Zip"],
			Subdivision:      record["SubdivisionName"],
			County:           record["County"],
			City:             record["City"],
			SchoolDistrict:   record["School"],
			LandValue:        record["Land_Value"],
			ImprovementValue: record["Improvement_Value"],
			TotalValue:       record["Total_Value"],
			DeedDate:         record["Deed_Date"],
			ARBIndicator:     record["ARB_Indicator"],
			YearBuilt:        record["Year_Built"],
			LivingArea:       record["Living_Area"],
			NumBedrooms:      record["Num_Bedrooms"],
			NumBathrooms:     record["Num_Bathrooms"],
			PropertyClass:    record["Property_Class"],
			StateUseCode:     record["State_Use_Code"],
			LandAcres:        record["Land_Acres"],
			LandSqFt:         record["Land_SqFt"],
		}
		mu2024.Lock()
		primary2024ByAcct[acct] = prop
		mu2024.Unlock()
	}); err != nil {
		return nil, nil, err
	}

	byAddress2024 := make(map[string]Property, len(primary2024ByAcct))
	for _, prop := range primary2024ByAcct {
		addrNorm := normalize(prop.SitusAddress)
		byAddress2024[addrNorm] = prop
	}

	return byAddress, byAddress2024, nil
}

// readFile iterates through a |-delimited file with a header row, calling fn for each record.
func readFile(path string, fn func(record map[string]string)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024) // allow very long lines

	// Read header row
	if !scanner.Scan() {
		return fmt.Errorf("file %s is empty", path)
	}
	header := strings.Split(scanner.Text(), "|")

	// Pipeline: producer (I/O) -> workers (CPU-bound parsing)
	linesCh := make(chan string, 4096)

	workers := runtime.NumCPU()
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for line := range linesCh {
				cols := strings.Split(line, "|")
				rec := make(map[string]string, len(header))
				for j, h := range header {
					if j < len(cols) {
						rec[h] = strings.TrimSpace(cols[j])
					}
				}
				fn(rec)
			}
		}()
	}

	for scanner.Scan() {
		linesCh <- scanner.Text()
	}
	close(linesCh)
	wg.Wait()

	return scanner.Err()
}

// normalize produces a canonical form of an address key.
func normalize(addr string) string {
	addr = strings.ToUpper(strings.TrimSpace(addr))
	addr = strings.ReplaceAll(addr, ",", "")
	addr = strings.Join(strings.Fields(addr), " ") // collapse whitespace
	return addr
}

// renderProperty prints the property information in a pleasant, readable layout.
func renderPropertyDiff(cur Property, prev Property) {
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
	Property
	NeighborCount int
	Mean          float64
	StdDev        float64
}

// findUndervaluedInSubdivision returns properties in the given subdivision whose ImprovementValue
// is at least one standard deviation below neighboring comps (0.25 mi).
func findUndervaluedInSubdivision(sub string, props map[string]Property) []undervaluedResult {
	sub = strings.ToUpper(strings.TrimSpace(sub))
	// Collect candidates in subdivision with coords & value.
	var candidates []Property
	for _, p := range props {
		if strings.ToUpper(strings.TrimSpace(p.Subdivision)) == sub && p.Latitude != "" && p.Longitude != "" && p.ImprovementValue != "" {
			candidates = append(candidates, p)
		}
	}
	return undervaluedFromCandidates(candidates, props)
}

// undervaluedFromCandidates runs the spatial+stat comparison for a set of candidate
// properties and returns those that are at least one standard deviation under the mean.
func undervaluedFromCandidates(candidates []Property, universe map[string]Property) []undervaluedResult {
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
