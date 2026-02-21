#!/usr/bin/env python3
"""generate_golden.py — Generate golden JSON values using herbie/cfgrib.

Usage:
    pip install herbie-data cfgrib numpy
    python testdata/generate_golden.py > testdata/golden_tmp700mb.json

This downloads the same HRRR GRIB2 message used in the Go tests and extracts
values at the reference points using cfgrib's nearest-neighbour lookup, then
writes a JSON file that the Go fixture test can validate against.
"""

import json
import sys
import os
import urllib.request

# ---------------------------------------------------------------------------
# Reference constants (must match hrrr_test.go)
# ---------------------------------------------------------------------------
GRIB_URL = "https://noaa-hrrr-bdp-pds.s3.amazonaws.com/hrrr.20260219/conus/hrrr.t12z.wrfsfcf00.grib2"
BYTE_START = 11928132
BYTE_END = 12500283

POINTS = [
    {"name": "Vail Pass CO",  "lat": 39.54, "lon": -106.19},
    {"name": "Denver CO",     "lat": 39.74, "lon": -104.98},
    {"name": "Seattle WA",    "lat": 47.61, "lon": -122.33},
]

FIXTURE_PATH = os.path.join(os.path.dirname(__file__), "hrrr_tmp700mb.grib2")
GOLDEN_PATH  = os.path.join(os.path.dirname(__file__), "golden_tmp700mb.json")


def download_fixture():
    """Download the GRIB2 byte range if not already present."""
    if os.path.exists(FIXTURE_PATH) and os.path.getsize(FIXTURE_PATH) > 500_000:
        print(f"  fixture already present ({os.path.getsize(FIXTURE_PATH):,} bytes)", file=sys.stderr)
        return
    req = urllib.request.Request(
        GRIB_URL,
        headers={"Range": f"bytes={BYTE_START}-{BYTE_END}"},
    )
    print(f"  downloading {BYTE_END - BYTE_START + 1:,} bytes from NOAA S3...", file=sys.stderr)
    with urllib.request.urlopen(req, timeout=120) as resp, open(FIXTURE_PATH, "wb") as f:
        f.write(resp.read())
    print(f"  wrote {os.path.getsize(FIXTURE_PATH):,} bytes to {FIXTURE_PATH}", file=sys.stderr)


def extract_values():
    """Use cfgrib to decode the fixture and look up reference point values."""
    try:
        import cfgrib
        import numpy as np
    except ImportError:
        print("ERROR: cfgrib and numpy required. pip install cfgrib numpy", file=sys.stderr)
        sys.exit(1)

    ds = cfgrib.open_dataset(FIXTURE_PATH, indexpath="")
    # cfgrib should give us 't' (temperature in K)
    t = ds["t"]
    lats = ds.coords["latitude"].values
    lons = ds.coords["longitude"].values  # signed -180..+180

    results = []
    for p in POINTS:
        # Nearest-neighbour: find flat index closest in Euclidean lat/lon distance.
        dlat = lats - p["lat"]
        dlon = lons - p["lon"]
        dist2 = dlat**2 + dlon**2
        idx = np.argmin(dist2)
        val = float(t.values.flat[idx])
        ij = np.unravel_index(idx, t.values.shape)
        results.append({
            "name":      p["name"],
            "lat":       p["lat"],
            "lon":       p["lon"],
            "value_K":   round(val, 6),
            "grid_i":    int(ij[1]),  # column = i
            "grid_j":    int(ij[0]),  # row    = j
        })
        print(f"  {p['name']}: {val:.6f} K  (i={ij[1]}, j={ij[0]})", file=sys.stderr)

    return results


def main():
    print("Generating golden values...", file=sys.stderr)
    download_fixture()
    results = extract_values()
    output = {
        "source": "herbie/cfgrib",
        "grib_url": GRIB_URL,
        "byte_start": BYTE_START,
        "byte_end": BYTE_END,
        "field": "TMP:700 mb",
        "run": "2026-02-19T12:00:00Z",
        "fxx": 0,
        "points": results,
    }
    json_str = json.dumps(output, indent=2)
    with open(GOLDEN_PATH, "w") as f:
        f.write(json_str + "\n")
    print(json_str)
    print(f"\nWrote {GOLDEN_PATH}", file=sys.stderr)


if __name__ == "__main__":
    main()
