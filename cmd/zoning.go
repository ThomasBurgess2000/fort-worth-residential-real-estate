package main

import (
	"fmt"
	"math"
	"path/filepath"

	shp "github.com/jonas-p/go-shp"
)

// zoningFeature represents a polygon (possibly multi-part) from the ADM_ZONING
// shapefile together with its associated attribute table values.
type zoningFeature struct {
	Parts  [][][2]float64    // Each part is a closed ring of [lat, lon] points
	Attrs  map[string]string // DBF attribute values keyed by field name
	MinLat float64
	MinLon float64
	MaxLat float64
	MaxLon float64
}

// Global slice containing all zoning polygons loaded at program start.
var zoningFeatures []zoningFeature

// initZoning loads the base zoning layer and any supplemental layers (e.g.
// overlay districts, PDs).  Add additional shapefile directories in the list
// below and they will all be searched.
func initZoning() error {
	layers := []struct {
		dir  string
		file string
	}{
		{"ADM_ZONING", "ADM_ZONING.shp"},
		{"ADM_ZONING_OVERLAY_DISTRICTS", "ADM_ZONING_OVERLAY_DISTRICTS.shp"},
	}

	for _, l := range layers {
		shpPath := filepath.Join("data", l.dir, l.file)
		feats, err := loadZoningShapefile(shpPath)
		if err != nil {
			return fmt.Errorf("load zoning shapefile %s: %w", shpPath, err)
		}
		zoningFeatures = append(zoningFeatures, feats...)
	}
	return nil
}

// loadZoningShapefile reads the shapefile at the given path and converts it to
// an in-memory slice of zoningFeature structs.
func loadZoningShapefile(path string) ([]zoningFeature, error) {
	r, err := shp.Open(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	fields := r.Fields()

	var features []zoningFeature
	for r.Next() {
		idx, shape := r.Shape()
		poly, ok := shape.(*shp.Polygon)
		if !ok {
			// Skip non-polygon geometries (shouldn't exist in zoning layer)
			continue
		}

		// Split the flat points slice into parts.
		numParts := len(poly.Parts)
		parts := make([][][2]float64, numParts)

		// Track bounding box while iterating.
		minLat, minLon := math.MaxFloat64, math.MaxFloat64
		maxLat, maxLon := -math.MaxFloat64, -math.MaxFloat64

		for partIdx := 0; partIdx < numParts; partIdx++ {
			start := poly.Parts[partIdx]
			end := int32(len(poly.Points))
			if partIdx+1 < numParts {
				end = poly.Parts[partIdx+1]
			}
			ring := make([][2]float64, int(end-start))
			j := 0
			for i := start; i < end; i++ {
				pt := poly.Points[i]
				ring[j] = [2]float64{pt.Y, pt.X} // lat, lon
				if pt.Y < minLat {
					minLat = pt.Y
				}
				if pt.Y > maxLat {
					maxLat = pt.Y
				}
				if pt.X < minLon {
					minLon = pt.X
				}
				if pt.X > maxLon {
					maxLon = pt.X
				}
				j++
			}
			parts[partIdx] = ring
		}

		attrs := make(map[string]string)
		for i, f := range fields {
			attrs[f.String()] = r.ReadAttribute(idx, i)
		}

		features = append(features, zoningFeature{
			Parts:  parts,
			Attrs:  attrs,
			MinLat: minLat,
			MinLon: minLon,
			MaxLat: maxLat,
			MaxLon: maxLon,
		})
	}
	return features, nil
}

// findZoningAttributes returns the attribute map for the first zoning polygon
// that contains the given lat/lon. The second return value is true if a match
// was found.
func findZoningAttributes(lat, lon float64) (map[string]string, bool) {
	for _, z := range zoningFeatures {
		if lat < z.MinLat || lat > z.MaxLat || lon < z.MinLon || lon > z.MaxLon {
			continue // quick bbox reject
		}
		for _, ring := range z.Parts {
			if pointInPolygon(lat, lon, ring) {
				return z.Attrs, true
			}
		}
	}
	return nil, false
}

// pointInPolygon implements the ray-casting algorithm for testing whether a
// point is inside a polygon. The polygon must be closed (first == last) but we
// don't require that here since shapefile rings are closed.
func pointInPolygon(lat, lon float64, ring [][2]float64) bool {
	inside := false
	j := len(ring) - 1
	for i := 0; i < len(ring); i++ {
		yi, xi := ring[i][0], ring[i][1]
		yj, xj := ring[j][0], ring[j][1]
		intersect := ((yi > lat) != (yj > lat)) && (lon < (xj-xi)*(lat-yi)/(yj-yi)+xi)
		if intersect {
			inside = !inside
		}
		j = i
	}
	return inside
}
