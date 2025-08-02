package main

import "strings"

// findPoorConditionInSubdivision returns all properties within the given subdivision
// whose Condition field equals "Poor" (case-insensitive).
func findPoorConditionInSubdivision(sub string, props map[string]Property) []Property {
	sub = strings.ToUpper(strings.TrimSpace(sub))
	var results []Property
	for _, p := range props {
		if strings.ToUpper(strings.TrimSpace(p.Subdivision)) == sub &&
			strings.EqualFold(strings.TrimSpace(p.Condition), "POOR") {
			results = append(results, p)
		}
	}
	return results
}
