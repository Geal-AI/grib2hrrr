package grib2hrrr

import (
	"encoding/binary"
	"fmt"
)

// DecodeMessage decodes a raw GRIB2 message (all sections) into a Field.
func DecodeMessage(raw []byte) (*Field, error) {
	// Verify GRIB indicator
	if _, err := parseSection0(raw); err != nil {
		return nil, err
	}

	// Walk sections; Section 0 is 16 bytes, Section 1 follows.
	off := 16 // skip Section 0

	var err error
	var grid *LambertGrid
	var drsTemplate = -1
	var drs0Params DRS0Params
	var drs53Params DRS53Params
	var hasDRS bool
	var sec7 []byte
	var bitmapData []byte // non-nil when Section 6 flag=0 (bitmap present)

	for off < len(raw) {
		// End marker
		if off+4 <= len(raw) && raw[off] == '7' && raw[off+1] == '7' && raw[off+2] == '7' && raw[off+3] == '7' {
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
			if len(sec) < 11 {
				return nil, fmt.Errorf("section 5 too short")
			}
			tmpl := int(binary.BigEndian.Uint16(sec[9:11]))
			switch tmpl {
			case 0:
				drs0Params, err = parseDRS0(sec)
				if err != nil {
					return nil, fmt.Errorf("section 5: %w", err)
				}
			case 3:
				drs53Params, err = parseDRS53(sec)
				if err != nil {
					return nil, fmt.Errorf("section 5: %w", err)
				}
			default:
				return nil, fmt.Errorf("unsupported DRS template %d (supported: 5.0, 5.3)", tmpl)
			}
			drsTemplate = tmpl
			hasDRS = true
		case 6:
			// Section 6: Bitmap
			if len(sec) < 6 {
				return nil, fmt.Errorf("section 6 too short")
			}
			switch sec[5] {
			case 255:
				// No bitmap — all grid points have data
			case 0:
				// Bitmap present in this section (MSB-first bit array follows header)
				bitmapData = sec[6:]
			default:
				return nil, fmt.Errorf("bitmap section: unsupported indicator %d", sec[5])
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

	var vals []float64
	switch drsTemplate {
	case 0:
		vals, err = unpackDRS0(sec7, drs0Params)
		if err != nil {
			return nil, fmt.Errorf("unpack DRS 5.0: %w", err)
		}
	case 3:
		vals, err = unpackDRS53(sec7, drs53Params)
		if err != nil {
			return nil, fmt.Errorf("unpack DRS 5.3: %w", err)
		}
	}

	// Expand values to the full Ni×Nj grid when a bitmap is present.
	// The unpack functions produce only N values (= number of set bits);
	// applyBitmap inserts NaN at positions where the bitmap bit is 0.
	if bitmapData != nil {
		vals, err = applyBitmap(vals, bitmapData, int(int64(grid.Ni)*int64(grid.Nj)))
		if err != nil {
			return nil, fmt.Errorf("applying bitmap: %w", err)
		}
	}

	// Issue #10: use int64 arithmetic for the product to avoid overflow on 32-bit platforms.
	expected64 := int64(grid.Ni) * int64(grid.Nj)
	if int64(len(vals)) != expected64 {
		return nil, fmt.Errorf("decoded %d values, expected %d (%dx%d)",
			len(vals), expected64, grid.Ni, grid.Nj)
	}

	return &Field{Grid: *grid, Vals: vals}, nil
}
