package main

// WGS-84 → Texas North-Central (EPSG:2276) Lambert Conformal Conic, US-feet.
// The shapefile polygons are already in this CRS (see ADM_ZONING.prj).
// We convert parcel lat/lon for point-in-polygon testing.

import "math"

const (
	spFalseEasting  = 1968500.0
	spFalseNorthing = 6561666.666666666
	phi0Deg         = 31.66666666666667 // latitude of origin
	phi1Deg         = 32.13333333333333 // standard parallel 1
	phi2Deg         = 33.96666666666667 // standard parallel 2
	lon0Deg         = -98.5             // central meridian

	ftPerMeter = 3.2808333333333334 // US survey foot
	semiMajorM = 6378137.0          // NAD83 semi-major axis (metres)
)

var (
	n    float64
	F    float64
	rho0 float64
)

func init() {
	phi1 := phi1Deg * math.Pi / 180
	phi2 := phi2Deg * math.Pi / 180
	phi0 := phi0Deg * math.Pi / 180

	const e2 = 0.00669438002290 // NAD83 eccentricity squared

	m := func(phi float64) float64 {
		return math.Cos(phi) / math.Sqrt(1-e2*math.Sin(phi)*math.Sin(phi))
	}

	t := func(phi float64) float64 {
		e := math.Sqrt(e2)
		return math.Tan(math.Pi/4-phi/2) / math.Pow((1-e*math.Sin(phi))/(1+e*math.Sin(phi)), e/2)
	}

	m1 := m(phi1)
	m2 := m(phi2)
	t1 := t(phi1)
	t2 := t(phi2)
	t0 := t(phi0)

	n = math.Log(m1/m2) / math.Log(t1/t2)

	aFt := semiMajorM * ftPerMeter
	F = aFt * m1 / (n * math.Pow(t1, n))
	rho0 = F * math.Pow(t0, n)
}

// wgs84ToTxNC converts latitude/longitude in decimal degrees (WGS-84) to
// State-Plane North-Central Texas Lambert feet. It returns (northingFT, eastingFT)
// which correspond to (lat, lon) ordering used in the zoningFeature rings.
func wgs84ToTxNC(latDeg, lonDeg float64) (northingFt, eastingFt float64) {
	phi := latDeg * math.Pi / 180
	lambda := lonDeg * math.Pi / 180
	lambda0 := lon0Deg * math.Pi / 180

	t := math.Tan(math.Pi/4 - phi/2)
	rho := F * math.Pow(t, n)
	theta := n * (lambda - lambda0)

	xFt := rho*math.Sin(theta) + spFalseEasting
	yFt := rho0 - rho*math.Cos(theta) + spFalseNorthing

	eastingFt = xFt
	northingFt = yFt
	return
}
