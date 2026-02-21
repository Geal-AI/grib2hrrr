package grib2hrrr_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/geal-ai/grib2hrrr"
)

// Reference: herbie/cfgrib, HRRR 2026-02-19 T12Z F00, TMP:700 mb
// Nearest-neighbour lookup at each point.
const (
	refGRIBURL   = "https://noaa-hrrr-bdp-pds.s3.amazonaws.com/hrrr.20260219/conus/hrrr.t12z.wrfsfcf00.grib2"
	refByteStart = int64(11928132)
	refByteEnd   = int64(12500283)
)

// refPoints are (lat°N, lon°E signed, expected_K, description).
var refPoints = []struct {
	lat, lon float64
	want     float64
	name     string
}{
	{39.54, -106.19, 259.061798, "Vail Pass CO"},
	{39.74, -104.98, 261.374298, "Denver CO"},
	{47.61, -122.33, 256.686798, "Seattle WA"},
}

// TestLambertGridParams verifies the HRRR grid constants.
// Requires network access to NOAA S3 — skipped in short mode.
func TestLambertGridParams(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: requires network access to NOAA S3")
	}
	c := grib2hrrr.NewHRRRClient()
	raw, err := c.FetchRaw(context.Background(), refGRIBURL, refByteStart, refByteEnd)
	if err != nil {
		t.Fatalf("FetchRaw: %v", err)
	}
	field, err := grib2hrrr.DecodeMessage(raw)
	if err != nil {
		t.Fatalf("DecodeMessage: %v", err)
	}

	g := field.Grid
	check := func(name string, got, want, tol float64) {
		t.Helper()
		if math.Abs(got-want) > tol {
			t.Errorf("%s: got %.6f, want %.6f (diff=%.6f)", name, got, want, math.Abs(got-want))
		}
	}

	if g.Ni != 1799 {
		t.Errorf("Ni: got %d, want 1799", g.Ni)
	}
	if g.Nj != 1059 {
		t.Errorf("Nj: got %d, want 1059", g.Nj)
	}
	check("La1", g.La1, 21.138123, 1e-5)
	check("Lo1", grib2hrrr.NormLon(g.Lo1), -122.719528, 1e-4)
	check("LoV", grib2hrrr.NormLon(g.LoV), -97.5, 1e-4)
	check("Latin1", g.Latin1, 38.5, 1e-4)
	check("Latin2", g.Latin2, 38.5, 1e-4)
	check("Dx", g.Dx, 3000.0, 1.0)
	check("Dy", g.Dy, 3000.0, 1.0)
	if g.ScanMode != 0x40 {
		t.Errorf("ScanMode: got 0x%02X, want 0x40", g.ScanMode)
	}
}

// TestGridIndices verifies LatLonToIJ against herbie's nearest-neighbour indices.
// Requires network access to NOAA S3 — skipped in short mode.
func TestGridIndices(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: requires network access to NOAA S3")
	}
	c := grib2hrrr.NewHRRRClient()
	raw, err := c.FetchRaw(context.Background(), refGRIBURL, refByteStart, refByteEnd)
	if err != nil {
		t.Fatalf("FetchRaw: %v", err)
	}
	field, err := grib2hrrr.DecodeMessage(raw)
	if err != nil {
		t.Fatalf("DecodeMessage: %v", err)
	}

	// Expected indices from herbie
	expected := []struct{ i, j int }{{651, 579}, {686, 584}, {278, 953}}

	for k, rp := range refPoints {
		ei, ej := expected[k].i, expected[k].j
		gi, gj := field.Grid.LatLonToIJ(rp.lat, rp.lon)
		if abs(gi-ei) > 1 || abs(gj-ej) > 1 {
			t.Errorf("%s: LatLonToIJ(%.2f,%.2f)=(%d,%d), want (%d,%d)",
				rp.name, rp.lat, rp.lon, gi, gj, ei, ej)
		}
	}
}

// TestFieldValues verifies decoded TMP 700mb values against Python/herbie reference.
// Requires network access to NOAA S3 — skipped in short mode.
func TestFieldValues(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: requires network access to NOAA S3")
	}
	c := grib2hrrr.NewHRRRClient()
	raw, err := c.FetchRaw(context.Background(), refGRIBURL, refByteStart, refByteEnd)
	if err != nil {
		t.Fatalf("FetchRaw: %v", err)
	}
	field, err := grib2hrrr.DecodeMessage(raw)
	if err != nil {
		t.Fatalf("DecodeMessage: %v", err)
	}

	const tol = 0.01 // K — should match to < 0.01 K (quantisation ≈ 0.001 K)
	for _, rp := range refPoints {
		got := field.Lookup(rp.lat, rp.lon)
		if math.IsNaN(got) {
			t.Errorf("%s: Lookup returned NaN", rp.name)
			continue
		}
		diff := math.Abs(got - rp.want)
		if diff > tol {
			t.Errorf("%s: got %.6f K, want %.6f K (diff=%.6f)", rp.name, got, rp.want, diff)
		} else {
			t.Logf("%s: %.6f K (expected %.6f, diff=%.6f) ✓", rp.name, got, rp.want, diff)
		}
	}
}

// TestCornerAndCenterValues verifies grid corner and center values.
// Requires network access to NOAA S3 — skipped in short mode.
func TestCornerAndCenterValues(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: requires network access to NOAA S3")
	}
	c := grib2hrrr.NewHRRRClient()
	raw, err := c.FetchRaw(context.Background(), refGRIBURL, refByteStart, refByteEnd)
	if err != nil {
		t.Fatalf("FetchRaw: %v", err)
	}
	field, err := grib2hrrr.DecodeMessage(raw)
	if err != nil {
		t.Fatalf("DecodeMessage: %v", err)
	}

	g := field.Grid
	cases := []struct {
		name string
		i, j int
		want float64
	}{
		{"SW corner (0,0)", 0, 0, 281.311798},
		{"grid center (899,529)", 899, 529, 272.311798},
	}

	const tol = 0.1
	for _, tc := range cases {
		idx := tc.j*g.Ni + tc.i
		if idx < 0 || idx >= len(field.Vals) {
			t.Errorf("%s: index %d out of range", tc.name, idx)
			continue
		}
		got := field.Vals[idx]
		diff := math.Abs(got - tc.want)
		if diff > tol {
			t.Errorf("%s: got %.6f K, want %.6f K (diff=%.6f)", tc.name, got, tc.want, diff)
		} else {
			t.Logf("%s: %.6f K ✓", tc.name, got)
		}
	}
}

// TestFetchField exercises the high-level FetchField API.
// Requires network access to NOAA S3 — always skipped in short mode.
func TestFetchField(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}
	c := grib2hrrr.NewHRRRClient()
	run := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	field, err := c.FetchField(context.Background(), run, 0, "TMP:700 mb")
	if err != nil {
		t.Fatalf("FetchField: %v", err)
	}
	// Spot check Vail Pass
	got := field.Lookup(39.54, -106.19)
	want := 259.061798
	if math.Abs(got-want) > 0.01 {
		t.Errorf("Vail Pass: got %.4f K, want %.4f K", got, want)
	}
	t.Logf("Vail Pass 700mb: %.4f K ✓", got)
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
