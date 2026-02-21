# grib2hrrr

A pure-Go library for fetching and decoding NOAA HRRR model output directly from S3 via HTTP range requests. No Python. No eccodes. No CGO.

```
go get github.com/geal-ai/grib2hrrr
```

## Why it exists

HRRR files are GRIB2, and GRIB2 decoders in Go either don't exist or rely on CGO bindings to `eccodes` (ECMWF's C library). HRRR uses two packing schemes that are rarely implemented correctly:

- **DRS Template 5.3** — complex packing with 2nd-order spatial differencing (used by temperature, reflectivity, and most analysis fields)
- **DRS Template 5.0** — simple packing (used by wind, precipitation, pressure, visibility, and others)

Both are implemented from the WMO GRIB2 specification. The library also handles **Section 6 bitmaps** — sparse fields like cloud ceiling that only have values where clouds are present.

Range requests mean only ~570 KB is fetched from a 200 MB file.

## Quick Start

### Library

```go
package main

import (
    "context"
    "fmt"
    "time"
    "github.com/geal-ai/grib2hrrr"
)

func main() {
    client := grib2hrrr.NewHRRRClient()
    ctx := context.Background()

    // Most recent available run (HRRR is published ~45 min after nominal run time)
    run := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Hour)

    // Fetch 700mb temperature at forecast hour 0
    field, err := client.FetchField(ctx, run, 0, "TMP:700 mb")
    if err != nil {
        panic(err)
    }

    // Look up temperature at any lat/lon on the Lambert conformal grid
    lat, lon := 39.54, -106.19 // Vail Pass, CO
    tempK := field.Lookup(lat, lon)

    fmt.Printf("700mb temp at Vail Pass: %.1f°C\n", tempK-273.15)
}
```

### CLI

```bash
go install github.com/geal-ai/grib2hrrr/cmd/hrrr@latest
```

```bash
# Current 2m temperature at a lat/lon (auto-detects latest run)
hrrr 39.64 -106.37

# Specific variable
hrrr -var "UGRD:10 m above ground" 39.64 -106.37

# Forecast hour 6
hrrr -var "REFC:entire atmosphere" -fxx 6 39.64 -106.37

# All 18 known variables at once
hrrr -all 39.64 -106.37

# JSON output (single variable)
hrrr -json 39.64 -106.37

# JSON output for all variables — pipe into jq, scripts, etc.
hrrr -all -json 39.64 -106.37 | jq '.fields[] | select(.values) | {v: .variable, t: .values.celsius}'

# Specific model run
hrrr -run 2026-02-21T12:00:00Z 39.64 -106.37

# List all supported variables
hrrr -list
```

Example `-all` output:

```
  Location : 39.6400°N  -106.3700°E
  Run      : 2026-02-21 13:00Z UTC
  Valid    : 2026-02-21 13:00Z UTC  [analysis (f00)]

  TMP:2 m above ground    256.27 K  /  -16.88 °C  /  1.6 °F
  TMP:surface             248.63 K  /  -24.52 °C  /  -12.1 °F
  UGRD:10 m above ground  0.66 m/s  /  1.5 mph
  VGRD:10 m above ground  -1.69 m/s  /  -3.8 mph
  PRES:surface            72410.0 Pa  /  724.10 hPa
  VIS:surface             28700 m  /  17.83 miles
  HGT:cloud ceiling       (no data)     ← clear sky: bitmap marks this point missing
  ...
```

Example `-json` output:

```json
{
  "location": { "lat": 39.64, "lon": -106.37 },
  "run": "2026-02-21T13:00:00Z",
  "valid": "2026-02-21T13:00:00Z",
  "fxx": 0,
  "fields": [
    {
      "variable": "TMP:2 m above ground",
      "values": { "raw": 256.27, "celsius": -16.88, "fahrenheit": 1.62 }
    }
  ]
}
```

## Supported Variables

All 18 variables work with `-var` or `-all`:

| Variable | Description |
|----------|-------------|
| `TMP:2 m above ground` | 2 m air temperature (K → °C / °F) |
| `TMP:surface` | Surface skin temperature |
| `TMP:700 mb` | 700 mb temperature |
| `TMP:500 mb` | 500 mb temperature |
| `DPT:2 m above ground` | 2 m dew point |
| `RH:2 m above ground` | 2 m relative humidity (%) |
| `REFC:entire atmosphere` | Composite reflectivity (dBZ) |
| `CAPE:surface` | Surface CAPE (J/kg) |
| `UGRD:10 m above ground` | 10 m U-wind (m/s → mph) |
| `VGRD:10 m above ground` | 10 m V-wind (m/s → mph) |
| `PRATE:surface` | Precipitation rate (kg/m²/s → in/hr) |
| `APCP:surface` | Accumulated precipitation (kg/m²) |
| `HGT:cloud ceiling` | Cloud ceiling height (m → ft) — NaN at clear-sky points |
| `VIS:surface` | Surface visibility (m → miles) |
| `PRES:surface` | Surface pressure (Pa → hPa) |
| `MSLMA:mean sea level` | Mean sea-level pressure (Pa → hPa) |
| `TCDC:entire atmosphere` | Total cloud cover (%) |
| `SPFH:2 m above ground` | 2 m specific humidity (kg/kg) |

Any variable in the HRRR index can be fetched by passing a substring of its index line to `-var` or `FetchField`. Run `hrrr -list` for the curated list or browse the index at `https://noaa-hrrr-bdp-pds.s3.amazonaws.com/` (e.g. `hrrr.20260201/conus/hrrr.t00z.wrfsfcf00.grib2.idx`).

## How It Works

### 1. Range request from NOAA S3

HRRR files on `s3://noaa-hrrr-bdp-pds` are ~200 MB each. Each GRIB2 file is accompanied by a `.idx` index listing byte offsets for every variable. The library fetches the index, finds the target variable's offset, and issues an HTTP `Range: bytes=N-M` request — retrieving only ~570 KB instead of the full file.

### 2. GDT Template 3.30 — Lambert conformal grid

HRRR uses a Lambert conformal conic projection (GDT 3.30). The library decodes the 67-byte grid definition and implements the forward/inverse Lambert projection. `Lookup(lat, lon)` converts geographic coordinates to grid indices via nearest-neighbour interpolation on the 1799×1059 CONUS grid.

### 3. DRS Template 5.0 — simple packing

Used by wind, precipitation, pressure, visibility, and others. Each value is a fixed-width unsigned integer unpacked as:

```
Y = (R + X × 2^E) / 10^D
```

where `R` is the reference value and `E`, `D` are binary/decimal scale factors from Section 5.

### 4. DRS Template 5.3 — complex packing with 2nd-order spatial differencing

Used by temperature, reflectivity, CAPE, and most analysis fields. Values are encoded as groups of variable bit-width integers with a 2nd-order spatial differencing step applied before packing. After reading group reference values, the WMO spec requires a byte-boundary alignment before reading group widths (WMO Note 6). Without `br.align()`, the widths section starts 4 bits off → catastrophic divergence.

### 5. Section 6 — bitmap

Some fields (e.g. cloud ceiling) are only defined where a phenomenon exists. A bitmap encodes which of the Ni×Nj grid points have packed values; the rest are set to `NaN`. The library expands the N packed values to the full grid using the bitmap, so `Lookup` returns `NaN` at missing points.

## The Bug

While validating against Python/herbie reference output, we found a consistent ~12°C offset in 700mb temperature. Root cause:

```go
// drs53.go — after reading group reference values
// (nBits=9 × NG=64,732 groups = 582,588 bits — not byte-aligned)
br.align() // WMO Note (6): must end on a byte boundary
// Without this, widths reads 4 bits off → catastrophic divergence
```

After adding `br.align()`, all test cases pass with **0.000000 K error** against herbie reference values.

## Test Results

Values validated against Python/herbie (HRRR run 2026-02-21T01Z, F00, TMP:700 mb):

| Location | Coordinates | grib2hrrr (K) | herbie (K) | Error (K) |
|----------|-------------|---------------|------------|-----------|
| Vail Pass, CO | 39.54, -106.19 | 261.4175 | 261.4175 | 0.000000 |
| Denver, CO | 39.74, -104.98 | 260.9175 | 260.9175 | 0.000000 |
| Vail Mountain, CO | 39.64, -106.37 | 261.2925 | 261.2925 | 0.000000 |
| Cross-validation | 40.00, -105.50 | 259.8831 | 259.8831 | 0.000000 |

## Package Structure

```
grib2hrrr/
├── hrrr.go        # HRRRClient, FetchField, DecodeMessage — S3 + decode pipeline
├── sections.go    # GRIB2 section parsers: S0 (indicator), S3 (grid), S5 (DRS header)
├── lambert.go     # GDT 3.30 Lambert conformal conic — projection + Lookup()
├── drs0.go        # DRS 5.0 simple packing decoder
├── drs53.go       # DRS 5.3 complex packing decoder — groups, spatial diff, scale
├── bitmap.go      # Section 6 bitmap expand — fills NaN at missing grid points
├── bitstream.go   # MSB-first bit reader with byte-boundary alignment (br.align())
└── cmd/hrrr/      # CLI: -var, -all, -json, -fxx, -run, -list flags
```

## Requirements

- Go 1.22+
- Zero external dependencies
- Network access to `noaa-hrrr-bdp-pds.s3.amazonaws.com` (public, no auth)

## License

MIT
