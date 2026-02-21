# CLAUDE.md — grib2hrrr

<!-- Inherits from: ../CLAUDE.md (geal-ai org config) -->
<!-- Global OMC orchestration: ~/.claude/CLAUDE.md -->

## Project Identity

**Name:** grib2hrrr
**Type:** Go library (pure stdlib, no external dependencies)
**Language:** Go 1.22
**Module:** `github.com/geal-ai/grib2hrrr`
**Purpose:** Decode NOAA HRRR GRIB2 weather data files — Lambert conformal grid (GDT 3.30) + complex packing with spatial differencing (DRS Template 5.3)

## ⚠️ Safety Classification

**This library MAY be used in health and safety applications** (aviation weather, emergency management, severe weather alerting). Treat it accordingly:

- **All changes require tests FIRST** (TDD). No exceptions.
- **Document every change** — what changed, why, what was tested, what reference data was used.
- **Do not break existing behaviour silently.** Any change to the Lambert projection math, DRS 5.3 unpacking, or scale formula must be validated against the Python/wgrib2 reference.
- **Fuzz tests must pass** before merging any parser change.
- **Cross-validate against two independent references** (wgrib2 AND Python herbie/cfgrib) before considering decoder changes correct.

## Build & Test

```bash
# Run all tests (includes network calls to NOAA S3)
go test ./... -v

# Run offline tests only (no network, uses committed fixtures)
go test ./... -v -short

# Run with race detector
go test -race ./...

# Fuzz the binary parser (run for at least 60 seconds before merging parser changes)
go test -fuzz=FuzzDecodeMessage -fuzztime=60s

# Generate test fixtures (run once on developer machine, commit outputs)
make generate-fixtures   # requires Python + herbie installed

# Cross-validate against wgrib2
make validate-wgrib2     # requires: conda install -c conda-forge wgrib2

# Cross-validate against Python herbie (CI uses this with random point sampling)
make validate-python     # requires: pip install herbie-data cfgrib xarray numpy

# Run benchmarks
go test -bench=. -benchmem
```

## Architecture

```
hrrr.go        — HRRRClient (HTTP fetch + byte-range), DecodeMessage entry point
sections.go    — GRIB2 section parsers: Section0 (indicator), Section3 (GDT 3.30), Section5 (DRS 5.3 params)
drs53.go       — DRS Template 5.3 unpacking: grouped complex packing + spatial differencing
bitstream.go   — Bit-level reader (MSB-first, big-endian bit order)
lambert.go     — Lambert conformal projection (GDT 3.30): LatLonToIJ, IjToLatLon, Lookup
hrrr_test.go   — Integration tests (live NOAA S3); offline tests in *_test.go files

testdata/
  hrrr.20260219.t12z.wrfsfcf00.tmp700mb.grib2  — 572KB single-message fixture (committed)
  golden_tmp700mb.json                           — Reference values from herbie/cfgrib (committed)
  generate_golden.py                             — Script to regenerate golden values (run once)
  validate_random.py                             — CI: random point sampling against Python reference
```

## Validation Strategy

**Two-reference rule:** Any decoder output must agree with BOTH:
1. `wgrib2` (NOAA canonical GRIB2 tool)
2. Python `herbie` + `cfgrib` (independent implementation)

**Tolerance:** ≤ 0.01 K for DRS 5.3 (complex packing). Larger discrepancies indicate a decoder bug.

**CI pipeline:**
1. `go test -short ./...` — offline unit tests + fixture-based tests (no network)
2. `python testdata/validate_random.py` — random point sampling, Python vs Go, asserts ≤ 0.01 K
3. `go test -fuzz=FuzzDecodeMessage -fuzztime=30s` — parser robustness

## Critical Rules

- **Never use `panic` in library code.** Panics from `DecodeMessage` or any sub-function crash callers. Return errors.
- **Validate all size fields from untrusted input** before allocation (`ng`, `Ni`, `Nj`, `total`, `sLen`).
- **Always propagate `context.Context`** through HTTP calls. Callers must be able to cancel.
- **`go test -short ./...` must pass with zero network access.** All S3-dependent tests must have `testing.Short()` guards.
- **No changes to `drs53.go`, `lambert.go`, or `sections.go` without updating golden fixtures** and running `make validate-python`.

## Environment

No special environment setup needed. The library uses only Go standard library.

For fixture generation and CI validation:
```
HERBIE_DATA_DIR=./testdata   # where herbie downloads files
```

## References

- **Org Config:** `../CLAUDE.md`
- **GRIB2 WMO standard:** FM 92 GRIB Edition 2, WMO No. 306
- **DRS Template 5.3:** WMO GRIB2 Manual on Codes, Section 5 Template 3
- **GDT 3.30:** Lambert Conformal, WMO GRIB2 Manual on Codes, Section 3 Template 30
- **HRRR product page:** https://www.nco.ncep.noaa.gov/pmb/products/hrrr/
- **NOAA HRRR on AWS:** https://registry.opendata.aws/noaa-hrrr-pds/
- **wgrib2 docs:** https://www.cpc.ncep.noaa.gov/products/wesley/wgrib2/
- **herbie docs:** https://herbie.readthedocs.io/
