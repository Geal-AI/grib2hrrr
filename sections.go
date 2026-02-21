package grib2hrrr

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Section0 is the GRIB2 Indicator Section (16 bytes).
type Section0 struct {
	Discipline  byte
	Edition     byte
	TotalLength uint64
}

// Section3 holds GDT 3.30 Lambert conformal grid parameters decoded from HRRR.
// HRRR uses a non-standard compact form: no basic-angle/subdivisions fields,
// but adds a LaD field between ResolutionFlags and LoV.
type Section3 struct {
	Grid LambertGrid
}

// DRS53Params holds all parameters from DRS Template 5.3.
type DRS53Params struct {
	ReferenceValue     float64
	BinaryScaleFactor  int
	DecimalScaleFactor int
	Nbits              int // bits per group reference value
	TypeOfValue        byte
	SplittingMethod    byte
	MissingMgmt        byte
	PrimaryMissing     float64
	SecondaryMissing   float64
	NG                 int // number of groups
	RefGroupWidth      int
	BitsGroupWidth     int
	RefGroupLength     uint32
	LengthIncrement    byte
	LenLastGroup       uint32
	BitsGroupLength    int
	OrderSpatialDiff   int
	NOctetsExtra       int
}

// Input sanity limits — all well above any real HRRR file values.
const (
	// maxNG: real HRRR files have ~3700 groups; cap at 4M to prevent OOM allocs.
	// Issue #1: memory exhaustion via unbounded ng allocation.
	maxNG = 1 << 22 // 4,194,304

	// maxGridDim: HRRR CONUS is 1799×1059. Cap at 30000 per dimension.
	// Issue #10: Ni/Nj bounds.
	maxGridDim = 30000

	// maxBitWidth: bit fields for group widths/lengths are 1-byte values (0-255),
	// but any value > 64 is nonsensical for a uint64 accumulator.
	maxBitWidth = 64
)

// parseSection0 decodes the 16-byte indicator section.
func parseSection0(b []byte) (Section0, error) {
	if len(b) < 16 {
		return Section0{}, fmt.Errorf("section 0: need 16 bytes, got %d", len(b))
	}
	if string(b[0:4]) != "GRIB" {
		return Section0{}, fmt.Errorf("section 0: missing GRIB magic: %q", b[0:4])
	}
	return Section0{
		Discipline:  b[6],
		Edition:     b[7],
		TotalLength: binary.BigEndian.Uint64(b[8:16]),
	}, nil
}

// sectionAt finds a section starting at byte offset off in buf.
// Returns (sectionLen, sectionNum, sectionData, nextOffset).
// The "7777" end marker is only 4 bytes, so it is checked before the 5-byte header guard.
func sectionAt(buf []byte, off int) (uint32, byte, []byte, int, error) {
	// Check for "7777" end marker first — it is only 4 bytes, not a normal section.
	if off+4 <= len(buf) && buf[off] == '7' && buf[off+1] == '7' && buf[off+2] == '7' && buf[off+3] == '7' {
		return 4, 8, buf[off : off+4], off + 4, nil
	}
	if off+5 > len(buf) {
		return 0, 0, nil, 0, fmt.Errorf("section header at %d: out of bounds (buf=%d)", off, len(buf))
	}
	sLen := binary.BigEndian.Uint32(buf[off : off+4])
	sNum := buf[off+4]
	// Issue #10: use uint64 arithmetic to avoid int overflow on 32-bit platforms.
	end64 := uint64(off) + uint64(sLen)
	if end64 > uint64(len(buf)) {
		return 0, 0, nil, 0, fmt.Errorf("section %d at %d: length %d overflows buffer %d",
			sNum, off, sLen, len(buf))
	}
	end := int(end64)
	return sLen, sNum, buf[off:end], end, nil
}

// parseSection3HRRR decodes the GDT 3.30 section using HRRR's compact layout.
// Template offsets (g = start of GDT data, i.e. section3[14:]):
//
//	g+0       shape of earth (=6)
//	g+1..15   radius/major/minor (all zero for shape=6)
//	g+16..19  Ni
//	g+20..23  Nj
//	g+24..27  La1 (µdeg)
//	g+28..31  Lo1 (µdeg, 0-360)
//	g+32      resolution flags
//	g+33..36  LaD (µdeg, latitude at which Dx/Dy are specified)
//	g+37..40  LoV (µdeg, 0-360)
//	g+41..44  Dx (mm → /1000 = metres)
//	g+45..48  Dy (mm → /1000 = metres)
//	g+49      projection centre flag
//	g+50      scanning mode
//	g+51..54  Latin1 (µdeg)
//	g+55..58  Latin2 (µdeg)
//	g+59..62  Lat SP of southern pole (µdeg)
//	g+63..66  Lon SP of southern pole (µdeg)
func parseSection3HRRR(sec []byte) (Section3, error) {
	// sec[0:4] = length, sec[4]=3, sec[5]=source, sec[6:10]=Npts, sec[10]=listLen,
	// sec[11]=listInterp, sec[12:14]=GDT number → template starts at sec[14]
	if len(sec) < 14+67 {
		return Section3{}, fmt.Errorf("section 3: too short (%d bytes)", len(sec))
	}
	g := sec[14:] // GDT data

	u32 := func(off int) uint32 { return binary.BigEndian.Uint32(g[off : off+4]) }

	ni := int(u32(16))
	nj := int(u32(20))

	// Issue #10: validate grid dimensions before use.
	if ni <= 0 || ni > maxGridDim || nj <= 0 || nj > maxGridDim {
		return Section3{}, fmt.Errorf("section 3: invalid grid dimensions %dx%d (max %d)",
			ni, nj, maxGridDim)
	}

	la1 := float64(int32(u32(24))) / 1e6
	lo1 := float64(u32(28)) / 1e6
	// g+32: resolution flags (skip)
	// g+33..36: LaD (skip, informational)
	lov := float64(u32(37)) / 1e6
	dx := float64(u32(41)) / 1e3 // mm → m
	dy := float64(u32(45)) / 1e3
	scanMode := g[50]
	latin1 := float64(int32(u32(51))) / 1e6
	latin2 := float64(int32(u32(55))) / 1e6

	// Issue #9: ScanMode is parsed but the grid operations assume 0x40.
	// Reject unsupported modes rather than silently returning wrong values.
	// ⚠️ Safety: wrong scan mode → wrong grid values in safety-critical applications.
	if scanMode != 0x40 {
		return Section3{}, fmt.Errorf("section 3: unsupported scan mode 0x%02X (only 0x40 supported)", scanMode)
	}

	return Section3{
		Grid: LambertGrid{
			Ni:       ni,
			Nj:       nj,
			La1:      la1,
			Lo1:      lo1,
			LoV:      lov,
			Latin1:   latin1,
			Latin2:   latin2,
			Dx:       dx,
			Dy:       dy,
			ScanMode: scanMode,
		},
	}, nil
}

// parseDRS53 decodes Section 5 with DRS Template 5.3.
func parseDRS53(sec []byte) (DRS53Params, error) {
	// sec[0:4]=len, sec[4]=5, sec[5:9]=N, sec[9:11]=template_num, sec[11:]=template data
	if len(sec) < 11+38 {
		return DRS53Params{}, fmt.Errorf("section 5 DRS 5.3: too short (%d bytes)", len(sec))
	}
	t := sec[11:] // template data

	refBits := binary.BigEndian.Uint32(t[0:4])
	R := math.Float32frombits(refBits)

	E := decodeScaleFactor(binary.BigEndian.Uint16(t[4:6]))
	D := decodeScaleFactor(binary.BigEndian.Uint16(t[6:8]))

	nBits := int(t[8])
	typeVal := t[9]
	split := t[10]
	missMgmt := t[11]

	primMiss := math.Float32frombits(binary.BigEndian.Uint32(t[12:16]))
	secMiss := math.Float32frombits(binary.BigEndian.Uint32(t[16:20]))

	ng := int(binary.BigEndian.Uint32(t[20:24]))

	// Issue #1/#3: validate ng before allocating slices sized by it.
	// A crafted ng=0xFFFFFFFF would cause ~96 GB of allocation.
	if ng < 1 || ng > maxNG {
		return DRS53Params{}, fmt.Errorf("section 5: ng=%d out of valid range [1, %d]", ng, maxNG)
	}

	refGroupWidth := int(t[24])
	bitsGroupWidth := int(t[25])
	refGroupLen := binary.BigEndian.Uint32(t[26:30])
	lenInc := t[30]
	lenLast := binary.BigEndian.Uint32(t[31:35])
	bitsGroupLen := int(t[35])
	orderSD := int(t[36])
	nOctetsExtra := int(t[37])

	// Validate bit widths — values > 64 are nonsensical for a uint64 accumulator.
	if nBits > maxBitWidth {
		return DRS53Params{}, fmt.Errorf("section 5: Nbits=%d exceeds %d", nBits, maxBitWidth)
	}
	if bitsGroupWidth > maxBitWidth {
		return DRS53Params{}, fmt.Errorf("section 5: BitsGroupWidth=%d exceeds %d", bitsGroupWidth, maxBitWidth)
	}
	if bitsGroupLen > maxBitWidth {
		return DRS53Params{}, fmt.Errorf("section 5: BitsGroupLength=%d exceeds %d", bitsGroupLen, maxBitWidth)
	}

	return DRS53Params{
		ReferenceValue:     float64(R),
		BinaryScaleFactor:  E,
		DecimalScaleFactor: D,
		Nbits:              nBits,
		TypeOfValue:        typeVal,
		SplittingMethod:    split,
		MissingMgmt:        missMgmt,
		PrimaryMissing:     float64(primMiss),
		SecondaryMissing:   float64(secMiss),
		NG:                 ng,
		RefGroupWidth:      refGroupWidth,
		BitsGroupWidth:     bitsGroupWidth,
		RefGroupLength:     refGroupLen,
		LengthIncrement:    lenInc,
		LenLastGroup:       lenLast,
		BitsGroupLength:    bitsGroupLen,
		OrderSpatialDiff:   orderSD,
		NOctetsExtra:       nOctetsExtra,
	}, nil
}

// decodeScaleFactor decodes a GRIB2 sign-magnitude 2-byte scale factor.
// MSB is the sign bit (1=negative), remaining 15 bits are magnitude.
func decodeScaleFactor(raw uint16) int {
	magnitude := int(raw & 0x7FFF)
	if raw&0x8000 != 0 {
		return -magnitude
	}
	return magnitude
}
