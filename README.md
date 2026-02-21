# grib2hrrr

A pure-Go library for fetching and decoding NOAA HRRR model output directly from S3 via HTTP range requests. No Python. No eccodes. No CGO.

```
go get github.com/geal-ai/grib2hrrr
```

## Why it exists

HRRR files are GRIB2, and GRIB2 decoders in Go either don't exist or rely on CGO bindings to `eccodes` (ECMWF's C library). HRRR specifically uses **DRS Template 5.3** — complex packing with 2nd-order spatial differencing — which is rarely implemented correctly even in established libraries.

This library implements DRS 5.3 from the WMO GRIB2 specification, fixes a bit-alignment bug that no reference implementation documents clearly, and wraps it in a minimal S3 client that fetches only the bytes needed (~570 KB of a 200 MB file).

## Quick Start

```go
package main

import (
    "fmt"
    "time"
    "github.com/geal-ai/grib2hrrr"
)

func main() {
    client := grib2hrrr.NewHRRRClient()

    // Most recent available run (HRRR is published ~45 min after nominal run time)
    run := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Hour)

    // Fetch 700mb temperature at forecast hour 0
    field, err := client.FetchField(run, 0, "TMP:700 mb")
    if err != nil {
        panic(err)
    }

    // Look up temperature at any lat/lon on the Lambert conformal grid
    lat, lon := 39.54, -106.19 // Vail Pass, CO
    tempK := field.Lookup(lat, lon)

    fmt.Printf("700mb temp at Vail Pass: %.1f°C\n", tempK-273.15)
}
```

## How It Works

### 1. Range request from NOAA S3

HRRR files on `s3://noaa-hrrr-bdp-pds` are ~200 MB each. Each GRIB2 file is accompanied by a `.idx` index listing byte offsets for every variable. The library fetches the index, finds the target variable's offset, and issues an HTTP `Range: bytes=N-M` request — retrieving only ~570 KB instead of the full file.

### 2. GDT Template 3.30 — Lambert conformal grid

HRRR uses a Lambert conformal conic projection (GDT 3.30). The library decodes the 67-byte grid definition — including `Lov`, `Latin1`, `Latin2`, and `LoV` — and implements the forward/inverse Lambert projection. `Lookup(lat, lon)` converts geographic coordinates to grid indices via nearest-neighbor interpolation on the 1799×1059 CONUS grid.

### 3. DRS Template 5.3 — complex packing with 2nd-order spatial differencing

DRS 5.3 encodes values as groups of variable bit-width integers, with a 2nd-order spatial differencing step applied before packing. Decoding requires reading three distinct packed sections in sequence:

1. **Group reference values** — one reference per group, encoded at `nBits` width
2. **Group widths** — bit width of each group's data, variable-length encoded
3. **Group lengths** — number of values in each group, variable-length encoded

After the reference values section, the WMO specification requires the bit stream to be aligned to the next byte boundary before reading widths (WMO Note 6). With `nBits=9` and 64,732 groups, that's 582,588 bits — not byte-aligned — so without `br.align()` the widths section starts 4 bits off, causing catastrophic divergence.

## The Bug

While validating against Python/herbie reference output, we found a consistent ~12°C offset in 700mb temperature — not a constant bias, but diverging values indicating a structural misread. Root cause:

```go
// drs53.go — after reading group reference values
// (nBits=9 × NG=64,732 groups = 582,588 bits — not byte-aligned)
br.align() // WMO Note (6): group reference values must end on a byte boundary
// Without this, widths section reads 4 bits off → catastrophic divergence
```

After adding `br.align()`, all test cases pass with **0.000000 K error** against herbie reference values.

## Test Results

All values validated against Python/herbie (HRRR run 2026-02-21T01Z, F00, TMP:700 mb):

| Location | Coordinates | grib2hrrr (K) | herbie (K) | Error (K) |
|----------|-------------|---------------|------------|-----------|
| Vail Pass, CO | 39.54, -106.19 | 261.4175 | 261.4175 | 0.000000 |
| Denver, CO | 39.74, -104.98 | 260.9175 | 260.9175 | 0.000000 |
| Vail Mountain, CO | 39.64, -106.37 | 261.2925 | 261.2925 | 0.000000 |
| Cross-validation | 40.00, -105.50 | 259.8831 | 259.8831 | 0.000000 |

```
cd grib2hrrr && go test -v
=== RUN   TestHRRRField_VailPass
--- PASS: TestHRRRField_VailPass (0.58s)
=== RUN   TestHRRRField_Denver
--- PASS: TestHRRRField_Denver (0.61s)
=== RUN   TestHRRRField_VailMountain
--- PASS: TestHRRRField_VailMountain (0.54s)
=== RUN   TestHRRRField_CrossValidation
--- PASS: TestHRRRField_CrossValidation (0.56s)
PASS
```

## Package Structure

```
grib2hrrr/
├── hrrr.go        # HRRRClient, FetchField — S3 index fetch + range request + decode pipeline
├── sections.go    # GRIB2 section parsers: S0 (indicator), S3 (grid), S5 (data representation)
├── lambert.go     # GDT 3.30 Lambert conformal conic — forward/inverse projection, Lookup()
├── drs53.go       # DRS 5.3 complex packing decoder — group reference values, widths, lengths,
│                  #   2nd-order spatial differencing restore, scale/offset application
├── bitstream.go   # MSB-first bit reader with byte-boundary alignment (br.align())
└── hrrr_test.go   # 4 test cases: Vail Pass, Denver, Vail Mountain, cross-validation
```

## Requirements

- Go 1.22+
- Zero external dependencies
- Network access to `noaa-hrrr-bdp-pds.s3.amazonaws.com` (public, no auth)

## License

MIT
