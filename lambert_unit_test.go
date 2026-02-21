package grib2hrrr_test

// Unit tests for Lambert conformal projection math.
// No network required. Issues: #11 (NormLon export), #15 (unit tests).

import (
	"math"
	"testing"

	"github.com/geal-ai/grib2hrrr"
)

// hrrrGrid returns the known HRRR CONUS Lambert conformal grid constants.
func hrrrGrid() grib2hrrr.LambertGrid {
	return grib2hrrr.LambertGrid{
		Ni:       1799,
		Nj:       1059,
		La1:      21.138123,
		Lo1:      237.280472, // GRIB2 0-360 convention
		LoV:      262.5,      // GRIB2 0-360 convention (-97.5° signed)
		Latin1:   38.5,
		Latin2:   38.5,
		Dx:       3000.0,
		Dy:       3000.0,
		ScanMode: 0x40,
	}
}

// TestNormLon verifies longitude normalisation from 0-360 to -180..+180.
// Issue #11: NormLon should be exported.
func TestNormLon(t *testing.T) {
	tests := []struct {
		lon  float64
		want float64
	}{
		{0, 0},
		{90, 90},
		{180, 180},
		{181, -179},
		{270, -90},
		{360, 0},
		{-10, -10},   // already signed, no change
		{-180, -180}, // boundary
	}
	for _, tc := range tests {
		got := grib2hrrr.NormLon(tc.lon)
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("NormLon(%.1f) = %.6f, want %.6f", tc.lon, got, tc.want)
		}
	}
}

// TestLatLonToIJKnownPoints verifies LatLonToIJ against herbie/cfgrib reference indices.
// These expected values are from the Python herbie validation that generated golden_tmp700mb.json.
func TestLatLonToIJKnownPoints(t *testing.T) {
	g := hrrrGrid()
	tests := []struct {
		name     string
		lat, lon float64
		ei, ej   int // expected (i,j) from herbie, tolerance ±1
	}{
		{"Vail Pass CO", 39.54, -106.19, 651, 579},
		{"Denver CO", 39.74, -104.98, 686, 584},
		{"Seattle WA", 47.61, -122.33, 278, 953},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gi, gj := g.LatLonToIJ(tc.lat, tc.lon)
			if absInt(gi-tc.ei) > 1 || absInt(gj-tc.ej) > 1 {
				t.Errorf("LatLonToIJ(%.2f, %.2f) = (%d,%d), want (%d,%d) ±1",
					tc.lat, tc.lon, gi, gj, tc.ei, tc.ej)
			}
		})
	}
}

// TestIJRoundtrip verifies that IjToLatLon(LatLonToIJ(lat,lon)) ≈ (lat,lon).
// A round-trip error > 1 grid cell indicates a bug in the projection math.
func TestIJRoundtrip(t *testing.T) {
	g := hrrrGrid()
	// Interior points well within the grid — avoid edges where rounding may push outside
	tests := []struct{ lat, lon float64 }{
		{39.54, -106.19},
		{39.74, -104.98},
		{47.61, -122.33},
		{35.00, -100.00},
		{45.00, -90.00},
	}
	for _, tc := range tests {
		i, j := g.LatLonToIJ(tc.lat, tc.lon)
		lat2, lon2 := g.IjToLatLon(i, j)
		// Tolerance: nearest-neighbour rounding adds up to ±0.5 grid cell = ±1500 m.
		// At 3 km/cell and ~111 km/degree latitude: ±0.5 cell ≈ ±0.014°.
		const tol = 0.02 // degrees
		if math.Abs(lat2-tc.lat) > tol || math.Abs(lon2-tc.lon) > tol {
			t.Errorf("roundtrip (%.4f,%.4f) → ij(%d,%d) → (%.4f,%.4f): lat err=%.4f lon err=%.4f",
				tc.lat, tc.lon, i, j, lat2, lon2,
				math.Abs(lat2-tc.lat), math.Abs(lon2-tc.lon))
		}
	}
}

// TestLookupOutOfBounds verifies that Lookup returns NaN for points outside the grid.
func TestLookupOutOfBounds(t *testing.T) {
	g := hrrrGrid()
	vals := make([]float64, g.Ni*g.Nj)
	for i := range vals {
		vals[i] = float64(i) // arbitrary values
	}

	outOfBounds := []struct {
		name     string
		lat, lon float64
	}{
		{"near equator", 0.0, -97.5},
		{"far north", 85.0, -97.5},
		{"far east", 39.0, 20.0},
		{"far west", 39.0, -170.0},
	}
	for _, tc := range outOfBounds {
		t.Run(tc.name, func(t *testing.T) {
			got := g.Lookup(tc.lat, tc.lon, vals)
			if !math.IsNaN(got) {
				t.Errorf("Lookup(%g, %g) = %g, want NaN for out-of-bounds point",
					tc.lat, tc.lon, got)
			}
		})
	}
}

// TestLookupInBounds verifies that Lookup returns the correct value for a known grid index.
func TestLookupInBounds(t *testing.T) {
	g := hrrrGrid()
	vals := make([]float64, g.Ni*g.Nj)
	// Set a sentinel value at (i=651, j=579) — the Vail Pass index
	const sentinel = 259.061798
	vals[579*g.Ni+651] = sentinel

	// Lookup at Vail Pass coordinates should land on (651,579)
	got := g.Lookup(39.54, -106.19, vals)
	if math.IsNaN(got) {
		t.Fatal("Lookup(Vail Pass): got NaN, point should be in-bounds")
	}
	// Allow for ±1 grid cell from nearest-neighbour rounding — just check non-zero
	if got == 0 && sentinel != 0 {
		t.Errorf("Lookup(Vail Pass): got 0, expected sentinel %.6f at or near (651,579)", sentinel)
	}
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
