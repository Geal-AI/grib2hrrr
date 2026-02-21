package grib2hrrr_test

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/geal-ai/grib2hrrr"
)

// goldenPoint mirrors the "points" entries in testdata/golden_tmp700mb.json.
type goldenPoint struct {
	Name    string  `json:"name"`
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	ValueK  float64 `json:"value_K"`
	GridI   int     `json:"grid_i"`
	GridJ   int     `json:"grid_j"`
}

// goldenFile mirrors the top-level structure of testdata/golden_tmp700mb.json.
type goldenFile struct {
	Source      string        `json:"source"`
	Field       string        `json:"field"`
	ToleranceK  float64       `json:"tolerance_K"`
	Points      []goldenPoint `json:"points"`
}

const (
	fixturePath = "testdata/hrrr_tmp700mb.grib2"
	goldenPath  = "testdata/golden_tmp700mb.json"
)

// TestFixtureDecodeAndValues decodes the committed GRIB2 fixture and validates
// decoded values against the golden JSON produced by herbie/cfgrib.
// This test runs in -short mode (no network required — fixture is committed).
// Issue #17: committed test fixture + golden JSON.
func TestFixtureDecodeAndValues(t *testing.T) {
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Skipf("fixture not present (%v); run 'make fixtures' to download", err)
	}
	field, err := grib2hrrr.DecodeMessage(raw)
	if err != nil {
		t.Fatalf("DecodeMessage: %v", err)
	}

	// Validate grid parameters.
	g := field.Grid
	if g.Ni != 1799 {
		t.Errorf("Ni: got %d, want 1799", g.Ni)
	}
	if g.Nj != 1059 {
		t.Errorf("Nj: got %d, want 1059", g.Nj)
	}
	if g.ScanMode != 0x40 {
		t.Errorf("ScanMode: got 0x%02X, want 0x40", g.ScanMode)
	}

	// Load golden values.
	goldenRaw, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Skipf("golden file not present (%v); run 'make golden' to generate", err)
	}
	var golden goldenFile
	if err := json.Unmarshal(goldenRaw, &golden); err != nil {
		t.Fatalf("parse golden JSON: %v", err)
	}
	tol := golden.ToleranceK
	if tol == 0 {
		tol = 0.01
	}

	for _, ref := range golden.Points {
		got := field.Lookup(ref.Lat, ref.Lon)
		if math.IsNaN(got) {
			t.Errorf("%s: Lookup(%.2f, %.2f) returned NaN", ref.Name, ref.Lat, ref.Lon)
			continue
		}
		diff := math.Abs(got - ref.ValueK)
		if diff > tol {
			t.Errorf("%s: got %.6f K, want %.6f K (diff=%.6f, tol=%.3f)",
				ref.Name, got, ref.ValueK, diff, tol)
		} else {
			t.Logf("%s: %.6f K  (expected %.6f, diff=%.6f) ✓", ref.Name, got, ref.ValueK, diff)
		}
	}
}

// TestFixtureGridIndices validates that LatLonToIJ returns the expected grid
// indices for the reference points (within ±1 cell of herbie's nearest-neighbour).
// Issue #17.
func TestFixtureGridIndices(t *testing.T) {
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Skipf("fixture not present; run 'make fixtures'")
	}
	field, err := grib2hrrr.DecodeMessage(raw)
	if err != nil {
		t.Fatalf("DecodeMessage: %v", err)
	}

	goldenRaw, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Skipf("golden file not present; run 'make golden'")
	}
	var golden goldenFile
	if err := json.Unmarshal(goldenRaw, &golden); err != nil {
		t.Fatalf("parse golden JSON: %v", err)
	}

	for _, ref := range golden.Points {
		gi, gj := field.Grid.LatLonToIJ(ref.Lat, ref.Lon)
		di := gi - ref.GridI
		dj := gj - ref.GridJ
		if di < 0 {
			di = -di
		}
		if dj < 0 {
			dj = -dj
		}
		if di > 1 || dj > 1 {
			t.Errorf("%s: LatLonToIJ(%.2f, %.2f) = (%d, %d), want (%d, %d) ±1",
				ref.Name, ref.Lat, ref.Lon, gi, gj, ref.GridI, ref.GridJ)
		} else {
			t.Logf("%s: (%d, %d) ✓", ref.Name, gi, gj)
		}
	}
}
