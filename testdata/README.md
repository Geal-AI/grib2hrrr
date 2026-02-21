# testdata

This directory contains committed test fixtures and validation scripts for `grib2hrrr`.

## Files

| File | Size | Description |
|------|------|-------------|
| `hrrr_tmp700mb.grib2` | ~572 KB | HRRR TMP:700 mb GRIB2 message (byte-range from NOAA S3) |
| `golden_tmp700mb.json` | ~1 KB | Reference values decoded by herbie/cfgrib |
| `generate_golden.py` | — | Regenerates `golden_tmp700mb.json` from cfgrib |
| `validate_random.py` | — | CI: cross-validates Go decoder vs cfgrib at random points |

## Fixture provenance

```
URL:    https://noaa-hrrr-bdp-pds.s3.amazonaws.com/hrrr.20260219/conus/hrrr.t12z.wrfsfcf00.grib2
Range:  bytes=11928132-12500283
Field:  TMP:700 mb
Run:    2026-02-19 T12Z  F00
```

The fixture was extracted with:

```bash
curl -o testdata/hrrr_tmp700mb.grib2 \
  -H "Range: bytes=11928132-12500283" \
  "https://noaa-hrrr-bdp-pds.s3.amazonaws.com/hrrr.20260219/conus/hrrr.t12z.wrfsfcf00.grib2"
```

Or via `make fixtures`.

## Validation strategy

Two complementary validation methods are used (see CLAUDE.md safety directive):

### 1. Go fixture test (fast, offline)

`TestFixture` (in `fixture_test.go`) decodes `hrrr_tmp700mb.grib2` with the Go
library and compares values at three reference points against `golden_tmp700mb.json`.
Runs as part of `go test ./...` in short mode — no network required.

```bash
go test -run TestFixture ./...
```

### 2. Python cross-validation (CI, network optional)

`validate_random.py` decodes the same fixture with cfgrib (Python) and with the
Go decoder, then compares 100–200 random interior CONUS points to < 0.01 K tolerance.

```bash
pip install cfgrib numpy
python testdata/validate_random.py --n-points 100
```

Or via `make validate-python`.

## Regenerating golden values

If the fixture or reference implementation changes:

```bash
pip install cfgrib numpy eccodes herbie-data
python testdata/generate_golden.py
git add testdata/golden_tmp700mb.json
git commit -m "test: regenerate golden values"
```

Or via `make golden`.
