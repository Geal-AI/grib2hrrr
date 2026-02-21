// Command hrrr fetches HRRR fields and prints values at a lat/lon.
//
// Usage:
//
//	hrrr [flags] <lat> <lon>
//	hrrr -list
//
// Examples:
//
//	hrrr 39.64 -106.37
//	hrrr -var "TMP:2 m above ground" 40.71 -74.01
//	hrrr -var "REFC:entire atmosphere" -fxx 1 39.64 -106.37
//	hrrr -all 39.64 -106.37
//	hrrr -all -json 39.64 -106.37
//	hrrr -json 39.64 -106.37
//	hrrr -list
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/geal-ai/grib2hrrr"
)

// knownVars is the help text for -list.
// supported=true means this variable is confirmed to work (DRS Template 5.0 or 5.3).
var knownVars = []struct {
	key   string
	desc  string
	drs53 bool
}{
	{"TMP:2 m above ground", "2 m air temperature (K → °C / °F)", true},
	{"TMP:surface", "Surface skin temperature (K → °C / °F)", true},
	{"TMP:700 mb", "700 mb temperature (K → °C / °F)", true},
	{"TMP:500 mb", "500 mb temperature (K → °C / °F)", true},
	{"DPT:2 m above ground", "2 m dew point (K → °C / °F)", true},
	{"RH:2 m above ground", "2 m relative humidity (%)", true},
	{"REFC:entire atmosphere", "Composite reflectivity (dBZ)", true},
	{"CAPE:surface", "Surface CAPE (J/kg)", true},
	{"UGRD:10 m above ground", "10 m U-component of wind (m/s → mph)", true},
	{"VGRD:10 m above ground", "10 m V-component of wind (m/s → mph)", true},
	{"PRATE:surface", "Precipitation rate (kg/m²/s → in/hr)", true},
	{"APCP:surface", "Total accumulated precipitation (kg/m²)", true},
	{"HGT:cloud ceiling", "Cloud ceiling height (m → ft)", true},
	{"VIS:surface", "Surface visibility (m → miles)", true},
	{"PRES:surface", "Surface pressure (Pa → hPa)", true},
	{"MSLMA:mean sea level", "Mean sea level pressure (Pa → hPa)", true},
	{"TCDC:entire atmosphere", "Total cloud cover (%)", true},
	{"SPFH:2 m above ground", "2 m specific humidity (kg/kg)", true},
}

// jsonLocation is the location sub-object in JSON output.
type jsonLocation struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// jsonField is a single variable result in JSON output.
type jsonField struct {
	Variable string             `json:"variable"`
	Values   map[string]float64 `json:"values,omitempty"`
	Error    string             `json:"error,omitempty"`
}

// jsonOutput is the top-level JSON response.
type jsonOutput struct {
	Location jsonLocation `json:"location"`
	Run      string       `json:"run"`
	Valid    string       `json:"valid"`
	Fxx      int          `json:"fxx"`
	Fields   []jsonField  `json:"fields"`
}

// varResult holds one fetched variable's outcome for -all processing.
type varResult struct {
	key string
	val float64
	err error
}

func main() {
	varLevel := flag.String("var", "TMP:2 m above ground", "HRRR variable/level string (see -list)")
	fxx := flag.Int("fxx", 0, "Forecast hour (0 = analysis/current conditions, max 48)")
	runStr := flag.String("run", "", "Model run time UTC, e.g. 2026-02-21T12:00Z (default: auto-detect latest)")
	listVars := flag.Bool("list", false, "Print common variable strings and exit")
	allVars := flag.Bool("all", false, "Fetch and display all known variables")
	asJSON := flag.Bool("json", false, "Output results as JSON")
	flag.Usage = usage
	flag.Parse()

	if *listVars {
		printVarList()
		os.Exit(0)
	}

	if flag.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "error: lat and lon are required")
		usage()
		os.Exit(2)
	}

	lat, err := strconv.ParseFloat(flag.Arg(0), 64)
	if err != nil {
		fatalf("invalid lat %q: %v", flag.Arg(0), err)
	}
	lon, err := strconv.ParseFloat(flag.Arg(1), 64)
	if err != nil {
		fatalf("invalid lon %q: %v", flag.Arg(1), err)
	}

	var runTime time.Time
	if *runStr != "" {
		runTime, err = time.Parse(time.RFC3339, *runStr)
		if err != nil {
			fatalf("invalid -run %q: use RFC3339, e.g. 2026-02-21T12:00:00Z", *runStr)
		}
		runTime = runTime.UTC().Truncate(time.Hour)
	}

	client := grib2hrrr.NewHRRRClient()
	ctx := context.Background()

	if *allVars {
		runAll(ctx, client, runTime, *fxx, lat, lon, *asJSON)
	} else {
		runSingle(ctx, client, runTime, *fxx, *varLevel, lat, lon, *asJSON)
	}
}

// runSingle fetches and displays a single variable.
func runSingle(ctx context.Context, client *grib2hrrr.HRRRClient, runTime time.Time, fxx int, varLevel string, lat, lon float64, asJSON bool) {
	var field *grib2hrrr.Field
	var actualRun time.Time
	var err error

	if !runTime.IsZero() {
		field, err = fetchWithCtx(ctx, client, runTime, fxx, varLevel)
		if err != nil {
			fatalf("fetch failed: %v", err)
		}
		actualRun = runTime
	} else {
		field, actualRun, err = fetchLatest(ctx, client, fxx, varLevel)
		if err != nil {
			fatalf("could not find a recent HRRR run: %v", err)
		}
	}

	val := field.Lookup(lat, lon)
	if math.IsNaN(val) {
		fatalf("(%.4f, %.4f) is outside the HRRR CONUS domain", lat, lon)
	}

	if asJSON {
		validTime := actualRun.Add(time.Duration(fxx) * time.Hour)
		out := jsonOutput{
			Location: jsonLocation{Lat: lat, Lon: lon},
			Run:      actualRun.UTC().Format(time.RFC3339),
			Valid:    validTime.UTC().Format(time.RFC3339),
			Fxx:      fxx,
			Fields:   []jsonField{{Variable: varLevel, Values: valueMap(varLevel, val)}},
		}
		emitJSON(out)
	} else {
		printResult(lat, lon, actualRun, fxx, varLevel, val)
	}
}

// runAll fetches all known variables in parallel (up to 6 concurrent) and displays them.
func runAll(ctx context.Context, client *grib2hrrr.HRRRClient, runTime time.Time, fxx int, lat, lon float64, asJSON bool) {
	results := make([]varResult, len(knownVars))
	startIdx := 0

	var actualRun time.Time
	if !runTime.IsZero() {
		actualRun = runTime
	} else {
		// Detect the latest run by fetching the first variable; reuse the result.
		field, t, err := fetchLatest(ctx, client, fxx, knownVars[0].key)
		if err != nil {
			fatalf("could not find a recent HRRR run: %v", err)
		}
		actualRun = t
		val := field.Lookup(lat, lon)
		results[0] = varResult{key: knownVars[0].key, val: val}
		startIdx = 1
	}

	// Parallel fetch remaining variables with a concurrency cap of 6.
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for i := startIdx; i < len(knownVars); i++ {
		wg.Add(1)
		go func(idx int, key string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			field, err := fetchWithCtx(ctx, client, actualRun, fxx, key)
			if err != nil {
				results[idx] = varResult{key: key, err: err}
				return
			}
			results[idx] = varResult{key: key, val: field.Lookup(lat, lon)}
		}(i, knownVars[i].key)
	}
	wg.Wait()

	validTime := actualRun.Add(time.Duration(fxx) * time.Hour)

	if asJSON {
		fields := make([]jsonField, len(results))
		for i, r := range results {
			switch {
			case r.err != nil:
				fields[i] = jsonField{Variable: r.key, Error: r.err.Error()}
			case math.IsNaN(r.val):
				fields[i] = jsonField{Variable: r.key, Error: "outside HRRR CONUS domain"}
			default:
				fields[i] = jsonField{Variable: r.key, Values: valueMap(r.key, r.val)}
			}
		}
		emitJSON(jsonOutput{
			Location: jsonLocation{Lat: lat, Lon: lon},
			Run:      actualRun.UTC().Format(time.RFC3339),
			Valid:    validTime.UTC().Format(time.RFC3339),
			Fxx:      fxx,
			Fields:   fields,
		})
		return
	}

	// Text output: single header, then one line per variable.
	forecastLabel := "analysis (f00)"
	if fxx > 0 {
		forecastLabel = fmt.Sprintf("f%02d (+%dh forecast)", fxx, fxx)
	}
	maxKey := 0
	for _, v := range knownVars {
		if len(v.key) > maxKey {
			maxKey = len(v.key)
		}
	}
	fmt.Printf("\n")
	fmt.Printf("  Location : %.4f°N  %.4f°E\n", lat, lon)
	fmt.Printf("  Run      : %s UTC\n", actualRun.Format("2006-01-02 15:04Z"))
	fmt.Printf("  Valid    : %s UTC  [%s]\n", validTime.Format("2006-01-02 15:04Z"), forecastLabel)
	fmt.Printf("\n")
	for _, r := range results {
		switch {
		case r.err != nil:
			fmt.Printf("  %-*s  error: %v\n", maxKey, r.key, r.err)
		case math.IsNaN(r.val):
			fmt.Printf("  %-*s  (outside domain)\n", maxKey, r.key)
		default:
			fmt.Printf("  %-*s  %s\n", maxKey, r.key, formatValue(r.key, r.val))
		}
	}
	fmt.Printf("\n")
}

// fetchLatest tries model runs from 1h ago back to 6h ago, returning the first that succeeds.
func fetchLatest(ctx context.Context, client *grib2hrrr.HRRRClient, fxx int, varLevel string) (*grib2hrrr.Field, time.Time, error) {
	base := time.Now().UTC().Truncate(time.Hour)
	var lastErr error
	for lag := 1; lag <= 6; lag++ {
		t := base.Add(-time.Duration(lag) * time.Hour)
		fmt.Fprintf(os.Stderr, "trying run %s (-%dh lag)…\n", t.Format("2006-01-02 15Z"), lag)
		f, err := fetchWithCtx(ctx, client, t, fxx, varLevel)
		if err == nil {
			return f, t, nil
		}
		fmt.Fprintf(os.Stderr, "  not available: %v\n", err)
		lastErr = err
	}
	return nil, time.Time{}, lastErr
}

func fetchWithCtx(ctx context.Context, client *grib2hrrr.HRRRClient, t time.Time, fxx int, varLevel string) (*grib2hrrr.Field, error) {
	tctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return client.FetchField(tctx, t, fxx, varLevel)
}

// formatValue returns a human-readable string for a variable value with unit conversions.
func formatValue(varLevel string, val float64) string {
	prefix := strings.SplitN(varLevel, ":", 2)[0]
	switch prefix {
	case "TMP", "DPT":
		c := val - 273.15
		f := c*9/5 + 32
		return fmt.Sprintf("%.2f K  /  %.2f °C  /  %.1f °F", val, c, f)
	case "UGRD", "VGRD", "WIND":
		mph := val * 2.23694
		return fmt.Sprintf("%.2f m/s  /  %.1f mph", val, mph)
	case "PRATE":
		inhr := val * 141732.28
		return fmt.Sprintf("%.6f kg/m²/s  /  %.4f in/hr", val, inhr)
	case "HGT":
		ft := val * 3.28084
		return fmt.Sprintf("%.1f m  /  %.0f ft", val, ft)
	case "VIS":
		mi := val / 1609.344
		return fmt.Sprintf("%.0f m  /  %.2f miles", val, mi)
	case "PRES", "MSLMA":
		hpa := val / 100
		return fmt.Sprintf("%.1f Pa  /  %.2f hPa", val, hpa)
	case "REFC":
		return fmt.Sprintf("%.0f dBZ", val)
	default:
		return fmt.Sprintf("%g", val)
	}
}

// valueMap returns a map of named unit-converted values for JSON output.
func valueMap(varLevel string, val float64) map[string]float64 {
	prefix := strings.SplitN(varLevel, ":", 2)[0]
	m := map[string]float64{"raw": val}
	switch prefix {
	case "TMP", "DPT":
		m["celsius"] = val - 273.15
		m["fahrenheit"] = (val-273.15)*9/5 + 32
	case "UGRD", "VGRD", "WIND":
		m["mph"] = val * 2.23694
	case "PRATE":
		m["in_hr"] = val * 141732.28
	case "HGT":
		m["feet"] = val * 3.28084
	case "VIS":
		m["miles"] = val / 1609.344
	case "PRES", "MSLMA":
		m["hpa"] = val / 100
	}
	return m
}

// emitJSON writes jsonOutput to stdout as indented JSON.
func emitJSON(out jsonOutput) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fatalf("json encode: %v", err)
	}
}

// printResult displays the raw value and any applicable unit conversions.
func printResult(lat, lon float64, run time.Time, fxx int, varLevel string, val float64) {
	forecastLabel := "analysis (f00)"
	if fxx > 0 {
		forecastLabel = fmt.Sprintf("f%02d (+%dh forecast)", fxx, fxx)
	}
	validTime := run.Add(time.Duration(fxx) * time.Hour)

	fmt.Printf("\n")
	fmt.Printf("  Location : %.4f°N  %.4f°E\n", lat, lon)
	fmt.Printf("  Run      : %s UTC\n", run.Format("2006-01-02 15:04Z"))
	fmt.Printf("  Valid    : %s UTC  [%s]\n", validTime.Format("2006-01-02 15:04Z"), forecastLabel)
	fmt.Printf("  Variable : %s\n", varLevel)
	fmt.Printf("\n")
	fmt.Printf("  Value    : %s\n", formatValue(varLevel, val))
	fmt.Printf("\n")
}

func printVarList() {
	fmt.Println("Common HRRR variable strings for use with -var:")
	fmt.Println()
	fmt.Println("  ✅ = supported (DRS 5.0 and 5.3)")
	fmt.Println()
	maxKey := 0
	for _, v := range knownVars {
		if len(v.key) > maxKey {
			maxKey = len(v.key)
		}
	}
	for _, v := range knownVars {
		icon := "✅"
		if !v.drs53 {
			icon = "⚠️ "
		}
		fmt.Printf("  %s  %-*s  %s\n", icon, maxKey, v.key, v.desc)
	}
	fmt.Println()
	fmt.Println("The string must match a substring of a line in the HRRR .idx file.")
	fmt.Println("Browse all fields at: https://noaa-hrrr-bdp-pds.s3.amazonaws.com/")
	fmt.Println("  e.g. hrrr.20260101/conus/hrrr.t00z.wrfsfcf00.grib2.idx")
}

func usage() {
	fmt.Fprintln(os.Stderr, `hrrr — fetch HRRR fields and print values at a lat/lon

Usage:
  hrrr [flags] <lat> <lon>
  hrrr -list

Flags:`)
	flag.PrintDefaults()
	fmt.Fprintln(os.Stderr, `
Examples:
  hrrr 39.64 -106.37
  hrrr -var "TMP:2 m above ground" 40.71 -74.01
  hrrr -var "REFC:entire atmosphere" -fxx 1 39.64 -106.37
  hrrr -all 39.64 -106.37
  hrrr -all -json 39.64 -106.37
  hrrr -json 39.64 -106.37
  hrrr -run 2026-02-21T12:00:00Z 39.64 -106.37
  hrrr -list`)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
