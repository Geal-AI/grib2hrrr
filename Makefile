# Makefile for grib2hrrr
# Requires: Go 1.21+, Python 3.9+ with cfgrib+numpy for validation targets.

.PHONY: test test-short test-full vet lint fixtures validate-python fuzz help

## Default: short tests (no network)
all: test-short

# ---------------------------------------------------------------------------
# Go tests
# ---------------------------------------------------------------------------

## test-short: run all tests in -short mode (no network, ~200 ms)
test-short:
	go test -short -count=1 ./...

## test: alias for test-short
test: test-short

## test-full: run ALL tests including S3 network tests (requires internet)
test-full:
	go test -v -timeout 300s -count=1 ./...

## vet: run go vet
vet:
	go vet ./...

## lint: run golangci-lint (install: https://golangci-lint.run)
lint:
	golangci-lint run ./...

## cover: generate HTML coverage report
cover:
	go test -short -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# ---------------------------------------------------------------------------
# Fuzz targets (run briefly to catch regressions)
# ---------------------------------------------------------------------------

## fuzz: run all fuzz targets for 30s each
fuzz:
	go test -fuzz=FuzzDecodeMessage  -fuzztime=30s ./...
	go test -fuzz=FuzzUnpackDRS53   -fuzztime=30s ./...
	go test -fuzz=FuzzBitReaderRead -fuzztime=30s ./...

# ---------------------------------------------------------------------------
# Test fixtures
# ---------------------------------------------------------------------------

## fixtures: download committed GRIB2 fixture from NOAA S3 (if missing)
fixtures: testdata/hrrr_tmp700mb.grib2

testdata/hrrr_tmp700mb.grib2:
	@echo "Downloading HRRR TMP:700mb fixture from NOAA S3..."
	@mkdir -p testdata
	curl -f -o $@ \
	  -H "Range: bytes=11928132-12500283" \
	  "https://noaa-hrrr-bdp-pds.s3.amazonaws.com/hrrr.20260219/conus/hrrr.t12z.wrfsfcf00.grib2"
	@echo "Downloaded $$(wc -c < $@) bytes"

## golden: generate golden JSON values using cfgrib (requires fixtures + Python deps)
golden: fixtures
	python3 testdata/generate_golden.py

# ---------------------------------------------------------------------------
# Python cross-validation (CI: validate Go decoder vs cfgrib reference)
# ---------------------------------------------------------------------------

## validate-python: validate N random points vs cfgrib (requires fixtures + Python deps)
validate-python: fixtures
	python3 testdata/validate_random.py --n-points 100 --tol 0.01

## validate-python-ci: same but exit 1 on failure (for CI pipelines)
validate-python-ci: fixtures
	python3 testdata/validate_random.py --n-points 200 --tol 0.01

## install-python-deps: install Python validation dependencies
install-python-deps:
	pip3 install cfgrib numpy eccodes

# ---------------------------------------------------------------------------
# Fixture test: decode fixture with Go and compare against golden JSON
# ---------------------------------------------------------------------------

## test-fixture: decode committed fixture and validate against golden_tmp700mb.json
test-fixture: testdata/hrrr_tmp700mb.grib2 testdata/golden_tmp700mb.json
	go test -v -run TestFixture ./...

# ---------------------------------------------------------------------------
# Help
# ---------------------------------------------------------------------------

help:
	@echo ""
	@echo "grib2hrrr make targets:"
	@echo "  test-short        Run unit tests (no network, ~200 ms)"
	@echo "  test-full         Run all tests including S3 network tests"
	@echo "  vet               Run go vet"
	@echo "  lint              Run golangci-lint"
	@echo "  cover             HTML coverage report"
	@echo "  fuzz              Run fuzz targets (30s each)"
	@echo "  fixtures          Download committed GRIB2 fixture"
	@echo "  golden            Generate golden JSON from cfgrib"
	@echo "  validate-python   Cross-validate Go vs cfgrib (50 random points)"
	@echo "  validate-python-ci  Same, 200 points, for CI"
	@echo "  test-fixture      Decode fixture and compare against golden JSON"
	@echo "  install-python-deps  Install Python validation deps"
	@echo ""
