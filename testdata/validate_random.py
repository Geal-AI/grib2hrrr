#!/usr/bin/env python3
"""validate_random.py — Cross-validate grib2hrrr Go decoder against cfgrib.

This script:
  1. Reads the committed GRIB2 fixture file
  2. Decodes it with cfgrib (Python reference)
  3. Calls the Go test binary to get values for random sample points
  4. Compares and reports any discrepancies

Usage (from repo root):
    python testdata/validate_random.py [--n-points N] [--tol TOL_K]

Exit codes:
    0 — all points within tolerance
    1 — discrepancies found or error

Requires:
    pip install cfgrib numpy
    go test ./... (builds the test binary)

Environment variables:
    GRIB2HRRR_FIXTURE   — override path to .grib2 fixture (default: testdata/hrrr_tmp700mb.grib2)
    GRIB2HRRR_TOLERANCE — tolerance in Kelvin (default: 0.01)
    GRIB2HRRR_N_POINTS  — number of random interior points to sample (default: 50)
"""

import argparse
import json
import os
import random
import subprocess
import sys
import tempfile

FIXTURE_DEFAULT = os.path.join(os.path.dirname(__file__), "hrrr_tmp700mb.grib2")
TOL_DEFAULT = 0.01   # K — matches quantisation ≈ 0.001 K, well above floating point noise
N_DEFAULT   = 50

# HRRR CONUS grid bounds (approximate, degrees)
LAT_MIN, LAT_MAX =  21.2,  47.8
LON_MIN, LON_MAX = -122.7, -60.9


def parse_args():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--n-points", type=int, default=int(os.environ.get("GRIB2HRRR_N_POINTS", N_DEFAULT)))
    p.add_argument("--tol",      type=float, default=float(os.environ.get("GRIB2HRRR_TOLERANCE", TOL_DEFAULT)))
    p.add_argument("--fixture",  default=os.environ.get("GRIB2HRRR_FIXTURE", FIXTURE_DEFAULT))
    p.add_argument("--seed",     type=int, default=42)
    return p.parse_args()


def load_cfgrib(fixture_path):
    """Decode the fixture using cfgrib and return (lats, lons, values) arrays."""
    try:
        import cfgrib
        import numpy as np
    except ImportError:
        print("ERROR: cfgrib and numpy required. pip install cfgrib numpy", file=sys.stderr)
        sys.exit(1)

    ds = cfgrib.open_dataset(fixture_path, indexpath="")
    t = ds["t"]
    lats = ds.coords["latitude"].values
    lons = ds.coords["longitude"].values
    return lats, lons, t.values


def cfgrib_lookup(lats, lons, vals, lat, lon):
    """Nearest-neighbour lookup using cfgrib-decoded arrays."""
    import numpy as np
    dlat = lats - lat
    dlon = lons - lon
    dist2 = dlat**2 + dlon**2
    idx = int(np.argmin(dist2))
    ij = np.unravel_index(idx, vals.shape)
    return float(vals[ij])


def go_lookup(points):
    """Call the Go validation helper to look up values at the given points.

    Writes a JSON request to a temp file, runs `go run testdata/go_lookup_helper.go`,
    and reads back a JSON response.  Falls back to go test -run with a special env var.
    """
    # Use the simpler approach: write a Go program that uses the library.
    helper_src = _go_helper_src(points)
    with tempfile.TemporaryDirectory() as tmpdir:
        src = os.path.join(tmpdir, "main.go")
        with open(src, "w") as f:
            f.write(helper_src)
        result = subprocess.run(
            ["go", "run", src],
            capture_output=True,
            text=True,
            cwd=os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
            timeout=120,
        )
        if result.returncode != 0:
            print(f"ERROR: go run failed:\n{result.stderr}", file=sys.stderr)
            sys.exit(1)
        return json.loads(result.stdout)


def _go_helper_src(points):
    """Generate a small Go program that decodes the fixture and returns JSON values."""
    points_json = json.dumps(points)
    return f'''package main

import (
\t"encoding/json"
\t"fmt"
\t"os"

\t"github.com/geal-ai/grib2hrrr"
)

type point struct {{
\tLat float64 `json:"lat"`
\tLon float64 `json:"lon"`
}}

type result struct {{
\tLat   float64 `json:"lat"`
\tLon   float64 `json:"lon"`
\tValue float64 `json:"value"`
}}

func main() {{
\traw, err := os.ReadFile("testdata/hrrr_tmp700mb.grib2")
\tif err != nil {{
\t\tpanic(err)
\t}}
\tfield, err := grib2hrrr.DecodeMessage(raw)
\tif err != nil {{
\t\tpanic(err)
\t}}
\tvar pts []point
\tif err := json.Unmarshal([]byte(`{points_json}`), &pts); err != nil {{
\t\tpanic(err)
\t}}
\tresults := make([]result, len(pts))
\tfor i, p := range pts {{
\t\tresults[i] = result{{Lat: p.Lat, Lon: p.Lon, Value: field.Lookup(p.Lat, p.Lon)}}
\t}}
\tb, _ := json.Marshal(results)
\tfmt.Println(string(b))
}}
'''


def main():
    args = parse_args()
    random.seed(args.seed)

    if not os.path.exists(args.fixture):
        print(f"ERROR: fixture not found: {args.fixture}", file=sys.stderr)
        print("Run: python testdata/generate_golden.py  (or make fixtures)", file=sys.stderr)
        sys.exit(1)

    print(f"Loading cfgrib reference decoder...", file=sys.stderr)
    lats, lons, cf_vals = load_cfgrib(args.fixture)

    # Sample random interior points (within CONUS, avoiding edges)
    lat_margin = 1.0
    lon_margin = 2.0
    points = [
        {
            "lat": round(random.uniform(LAT_MIN + lat_margin, LAT_MAX - lat_margin), 4),
            "lon": round(random.uniform(LON_MIN + lon_margin, LON_MAX - lon_margin), 4),
        }
        for _ in range(args.n_points)
    ]

    print(f"Getting Go values for {len(points)} random points...", file=sys.stderr)
    go_results = go_lookup(points)

    failures = 0
    max_diff = 0.0
    for pt, go_r in zip(points, go_results):
        cf_val = cfgrib_lookup(lats, lons, cf_vals, pt["lat"], pt["lon"])
        go_val = go_r["value"]
        diff = abs(cf_val - go_val)
        if diff > max_diff:
            max_diff = diff
        if diff > args.tol:
            print(f"FAIL ({pt['lat']:.4f}, {pt['lon']:.4f}): "
                  f"go={go_val:.6f} K  cfgrib={cf_val:.6f} K  diff={diff:.6f} K  (tol={args.tol})",
                  file=sys.stderr)
            failures += 1

    total = len(points)
    print(f"\nResults: {total - failures}/{total} points within {args.tol} K tolerance",
          file=sys.stderr)
    print(f"Max diff: {max_diff:.6f} K", file=sys.stderr)

    if failures:
        print(f"FAIL: {failures} point(s) exceeded tolerance", file=sys.stderr)
        sys.exit(1)
    else:
        print("PASS", file=sys.stderr)
        sys.exit(0)


if __name__ == "__main__":
    main()
