package types

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