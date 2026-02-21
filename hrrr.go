package grib2hrrr

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Field is a decoded GRIB2 field: a Lambert conformal grid + float64 values.
// Values are stored row-major: vals[j*Grid.Ni + i].
type Field struct {
	Grid LambertGrid
	Vals []float64
}

// Lookup returns the nearest-neighbour value at (lat°N, lon°E).
func (f *Field) Lookup(lat, lon float64) float64 {
	return f.Grid.Lookup(lat, lon, f.Vals)
}

// HRRRClient fetches HRRR GRIB2 messages from the NOAA S3 bucket.
type HRRRClient struct {
	HTTPClient *http.Client
	BaseURL    string // default: "https://noaa-hrrr-bdp-pds.s3.amazonaws.com"
}

// NewHRRRClient returns a client with sensible defaults.
func NewHRRRClient() *HRRRClient {
	return &HRRRClient{
		HTTPClient: &http.Client{Timeout: 120 * time.Second},
		BaseURL:    "https://noaa-hrrr-bdp-pds.s3.amazonaws.com",
	}
}

// FetchField fetches and decodes a single GRIB2 field by variable/level.
// t is the model run time (UTC), fxx is the forecast hour (0-48).
// varLevel is an index search string, e.g. "TMP:700 mb".
func (c *HRRRClient) FetchField(t time.Time, fxx int, varLevel string) (*Field, error) {
	idxURL, gribURL := c.urls(t, fxx)

	// 1. Fetch index to find byte range
	byteStart, byteEnd, err := c.findByteRange(idxURL, varLevel)
	if err != nil {
		return nil, fmt.Errorf("index lookup %q: %w", varLevel, err)
	}

	// 2. Fetch the raw GRIB2 message bytes
	raw, err := c.fetchRange(gribURL, byteStart, byteEnd)
	if err != nil {
		return nil, fmt.Errorf("fetching GRIB2 bytes: %w", err)
	}

	// 3. Decode
	return DecodeMessage(raw)
}

// DecodeMessage decodes a raw GRIB2 message (all sections) into a Field.
func DecodeMessage(raw []byte) (*Field, error) {
	// Verify GRIB indicator
	if _, err := parseSection0(raw); err != nil {
		return nil, err
	}

	// Walk sections; Section 0 is 16 bytes, Section 1 follows.
	off := 16 // skip Section 0

	var grid *LambertGrid
	var drsParams DRS53Params
	var hasDRS bool
	var sec7 []byte

	for off < len(raw) {
		// End marker
		if off+4 <= len(raw) && string(raw[off:off+4]) == "7777" {
			break
		}
		sLen, sNum, sec, next, err := sectionAt(raw, off)
		if err != nil {
			return nil, err
		}
		_ = sLen

		switch sNum {
		case 1:
			// Section 1: Identification — skip
		case 2:
			// Section 2: Local use — skip
		case 3:
			s3, err := parseSection3HRRR(sec)
			if err != nil {
				return nil, fmt.Errorf("section 3: %w", err)
			}
			g := s3.Grid
			grid = &g
		case 4:
			// Section 4: Product definition — skip
		case 5:
			// Check template number
			if len(sec) < 11 {
				return nil, fmt.Errorf("section 5 too short")
			}
			tmpl := binary.BigEndian.Uint16(sec[9:11])
			if tmpl != 3 {
				return nil, fmt.Errorf("unsupported DRS template %d (only 5.3 supported)", tmpl)
			}
			drsParams, err = parseDRS53(sec)
			if err != nil {
				return nil, fmt.Errorf("section 5: %w", err)
			}
			hasDRS = true
		case 6:
			// Section 6: Bitmap — check for no-bitmap flag
			if len(sec) >= 6 && sec[5] != 255 {
				return nil, fmt.Errorf("bitmap sections not supported (flag=%d)", sec[5])
			}
		case 7:
			sec7 = sec
		}
		off = next
	}

	if grid == nil {
		return nil, fmt.Errorf("no Section 3 found in message")
	}
	if !hasDRS {
		return nil, fmt.Errorf("no Section 5 found in message")
	}
	if sec7 == nil {
		return nil, fmt.Errorf("no Section 7 found in message")
	}

	vals, err := unpackDRS53(sec7, drsParams)
	if err != nil {
		return nil, fmt.Errorf("unpack DRS 5.3: %w", err)
	}

	expected := grid.Ni * grid.Nj
	if len(vals) != expected {
		return nil, fmt.Errorf("decoded %d values, expected %d (%dx%d)",
			len(vals), expected, grid.Ni, grid.Nj)
	}

	return &Field{Grid: *grid, Vals: vals}, nil
}

// urls returns the index and GRIB2 S3 URLs for a given model run.
func (c *HRRRClient) urls(t time.Time, fxx int) (idxURL, gribURL string) {
	t = t.UTC()
	dateStr := t.Format("20060102")
	hrStr := fmt.Sprintf("%02d", t.Hour())
	fxxStr := fmt.Sprintf("%02d", fxx)
	base := fmt.Sprintf("%s/hrrr.%s/conus/hrrr.t%sz.wrfsfcf%s",
		c.BaseURL, dateStr, hrStr, fxxStr)
	return base + ".grib2.idx", base + ".grib2"
}

// findByteRange parses the HRRR index file and returns the byte range for varLevel.
// varLevel is matched as a substring of the colon-delimited index line.
func (c *HRRRClient) findByteRange(idxURL, varLevel string) (int64, int64, error) {
	resp, err := c.HTTPClient.Get(idxURL)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, err
	}

	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	for i, line := range lines {
		if !strings.Contains(line, varLevel) {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 3 {
			continue
		}
		start, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		// End byte: start of next line - 1, or end of file
		var end int64
		if i+1 < len(lines) {
			nextParts := strings.Split(lines[i+1], ":")
			if len(nextParts) >= 2 {
				nextStart, err := strconv.ParseInt(nextParts[1], 10, 64)
				if err == nil {
					end = nextStart - 1
				}
			}
		}
		if end == 0 {
			end = math.MaxInt64 // last message: fetch to EOF
		}
		return start, end, nil
	}
	return 0, 0, fmt.Errorf("variable %q not found in index", varLevel)
}

// fetchRange does an HTTP range request and returns the bytes.
func (c *HRRRClient) fetchRange(url string, start, end int64) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if end == math.MaxInt64 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", start))
	} else {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

// FetchRaw fetches raw bytes for a variable using pre-known byte offsets.
// This is useful for testing with a fixed known range.
func (c *HRRRClient) FetchRaw(gribURL string, byteStart, byteEnd int64) ([]byte, error) {
	return c.fetchRange(gribURL, byteStart, byteEnd)
}
