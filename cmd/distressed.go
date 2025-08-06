package main

import (
	"acquisitions/internal/types"
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// ---------------- Distressed-property filter ----------------

type distressedResult struct {
	types.Property
	PriceRatio float64
	AgeGap     float64
	DeprGap    float64
	Flags      string
	NbhdCount  int
}

// handleSubdivisionQuery prompts the user to choose an analysis method and displays results.
func handleSubdivisionQuery(sub string) {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("\nSelect analysis for subdivision %s:\n  1) Relative Improvement (price per sqft vs nearby)\n  2) Distressed-Property Filter\n  3) List \"Poor\" Condition Properties\nChoice (1/2/3, default 1): ", sub)
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)
		if choice == "" || choice == "1" {
			startSub := time.Now()

			// Query database for subdivision properties
			properties, err := db.QuerySubdivisionProperties(sub)
			if err != nil {
				fmt.Printf("Error querying subdivision properties: %v\n", err)
				return
			}

			results := findUndervaluedInSubdivision(sub, properties)
			fmt.Printf("\nFound %d undervalued properties in subdivision %s (%v)\n", len(results), sub, time.Since(startSub).Truncate(time.Millisecond))
			var lines []string
			var addrs []string
			for _, r := range results {
				val, _ := parseDollar(r.ImprovementValue)
				line := fmt.Sprintf("%-40s | Imp: %9.0f | μ=%.0f σ=%.0f n=%d", r.SitusAddress, val, r.Mean, r.StdDev, r.NeighborCount)
				lines = append(lines, line)
				addrs = append(addrs, r.SitusAddress)
				fmt.Println(line)
			}
			fmt.Println("Use ↑/↓ and Enter for details, Esc to exit.")
			interactiveSelect(addrs, lines, true)
			return
		}
		if choice == "2" {
			startSub := time.Now()

			// Query database for subdivision properties
			properties, err := db.QuerySubdivisionProperties(sub)
			if err != nil {
				fmt.Printf("Error querying subdivision properties: %v\n", err)
				return
			}

			results := findDistressedInSubdivision(sub, properties)
			fmt.Printf("\nFound %d distressed properties in subdivision %s (%v)\n", len(results), sub, time.Since(startSub).Truncate(time.Millisecond))
			// Display and enable interactive selection.
			var lines []string
			var addrs []string
			for _, r := range results {
				priceSq, _ := parseDollar(r.TotalValue)
				living, _ := parseDollar(r.LivingArea)
				line := fmt.Sprintf("%-40s | $/sqft: %6.0f (%.0f%% of nbhd) | AgeGap: %2.0f | DeprGap: %3.0f | Flags: %s",
					r.SitusAddress, priceSq/living, r.PriceRatio*100, r.AgeGap, r.DeprGap, r.Flags)
				lines = append(lines, line)
				addrs = append(addrs, r.SitusAddress)
				fmt.Println(line)
			}
			fmt.Println("Use ↑/↓ and Enter for details, Esc to exit.")
			interactiveSelect(addrs, lines, true)
			return
		}
		if choice == "3" {
			startSub := time.Now()

			// Query database for subdivision properties
			properties, err := db.QuerySubdivisionProperties(sub)
			if err != nil {
				fmt.Printf("Error querying subdivision properties: %v\n", err)
				return
			}

			results := findPoorConditionInSubdivision(sub, properties)
			fmt.Printf("\nFound %d 'Poor' condition properties in subdivision %s (%v)\n", len(results), sub, time.Since(startSub).Truncate(time.Millisecond))
			var lines []string
			var addrs []string
			for _, p := range results {
				line := fmt.Sprintf("%-40s | Condition: %s", p.SitusAddress, p.Condition)
				lines = append(lines, line)
				addrs = append(addrs, p.SitusAddress)
				fmt.Println(line)
			}
			fmt.Println("Use ↑/↓ and Enter for details, Esc to exit.")
			interactiveSelect(addrs, lines, true)
			return
		}
		fmt.Println("Invalid choice – enter 1, 2, or 3.")
	}
}

// findDistressedInSubdivision implements the SQL-like distressed-property filter for a single subdivision.
func findDistressedInSubdivision(sub string, props []types.Property) []distressedResult {
	sub = strings.ToUpper(strings.TrimSpace(sub))

	// 1. Build neighborhood benchmarks
	type agg struct {
		sumPriceSqft float64
		sumYearBuilt float64
		sumDepr      float64
		count        int
	}
	aggs := make(map[string]*agg)

	for _, p := range props {
		nb := strings.ToUpper(strings.TrimSpace(p.Subdivision))
		if nb == "" {
			continue
		}
		a, ok := aggs[nb]
		if !ok {
			a = &agg{}
			aggs[nb] = a
		}
		total, ok1 := parseDollar(p.TotalValue)
		living, ok2 := parseDollar(p.LivingArea)
		if ok1 && ok2 && living > 0 {
			a.sumPriceSqft += total / living
		}
		if y, err := strconv.Atoi(strings.TrimSpace(p.YearBuilt)); err == nil {
			a.sumYearBuilt += float64(y)
		}
		if d, ok := parseDollar(p.DepreciationPercent); ok {
			a.sumDepr += d
		}
		a.count++
	}

	type stats struct {
		priceSqft float64
		yearBuilt float64
		depr      float64
		count     int
	}
	nbhdStats := make(map[string]stats, len(aggs))
	for nb, a := range aggs {
		if a.count == 0 {
			continue
		}
		nbhdStats[nb] = stats{
			priceSqft: a.sumPriceSqft / float64(a.count),
			yearBuilt: a.sumYearBuilt / float64(a.count),
			depr:      a.sumDepr / float64(a.count),
			count:     a.count,
		}
	}

	// 2. Evaluate each parcel in the subdivision
	var results []distressedResult
	now := time.Now()

	for _, p := range props {
		nb := strings.ToUpper(strings.TrimSpace(p.Subdivision))
		if nb != sub {
			continue
		}
		stat, ok := nbhdStats[nb]
		if !ok || stat.count < 10 {
			continue // unreliable comps
		}

		total, ok1 := parseDollar(p.TotalValue)
		living, ok2 := parseDollar(p.LivingArea)
		if !ok1 || !ok2 || living == 0 {
			continue
		}
		priceRatio := (total / living) / stat.priceSqft
		if priceRatio > 0.70 {
			continue // needs to be >=30% cheaper
		}

		// Age & depreciation gaps
		yearBuilt, errY := strconv.Atoi(strings.TrimSpace(p.YearBuilt))
		ageGap := 0.0
		if errY == nil {
			ageGap = stat.yearBuilt - float64(yearBuilt)
		}
		deprVal, _ := parseDollar(p.DepreciationPercent)
		deprGap := deprVal - stat.depr

		physFlag := strings.EqualFold(p.Condition, "Poor") || strings.EqualFold(p.Condition, "Fair") || deprVal >= 40
		if !(ageGap >= 20 || deprGap >= 15 || physFlag) {
			continue
		}

		// Ownership / finance distress signals
		flagAbsentee := 0
		if p.City != "" && !strings.Contains(strings.ToUpper(p.OwnerCityState), strings.ToUpper(p.City)) {
			flagAbsentee = 1
		}
		flagLongHold := 0
		if t, err := time.Parse("01-02-2006", p.DeedDate); err == nil {
			if now.Sub(t) >= 10*365*24*time.Hour {
				flagLongHold = 1
			}
		}
		flagTaxProtest := 0
		if strings.EqualFold(strings.TrimSpace(p.ARBIndicator), "Y") {
			flagTaxProtest = 1
		}
		flagTaxShock := 0
		// Query 2024 data for comparison
		if prev, err := db.QueryPropertyByAddress2024(normalize(p.SitusAddress)); err == nil && prev != nil {
			if prevVal, ok := parseDollar(prev.TotalValue); ok && prevVal > 0 && total > 1.15*prevVal {
				flagTaxShock = 1
			}
		}
		if flagAbsentee+flagLongHold+flagTaxProtest+flagTaxShock == 0 {
			continue
		}

		flagList := []string{}
		if flagAbsentee == 1 {
			flagList = append(flagList, "absentee")
		}
		if flagLongHold == 1 {
			flagList = append(flagList, "longHold")
		}
		if flagTaxShock == 1 {
			flagList = append(flagList, "taxShock")
		}
		if flagTaxProtest == 1 {
			flagList = append(flagList, "taxProtest")
		}
		if physFlag {
			flagList = append(flagList, "physical")
		}

		results = append(results, distressedResult{
			Property:   p,
			PriceRatio: priceRatio,
			AgeGap:     ageGap,
			DeprGap:    deprGap,
			Flags:      strings.Join(flagList, ","),
			NbhdCount:  stat.count,
		})
	}

	return results
}
