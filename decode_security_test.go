package grib2hrrr

// Internal security regression tests — no network required.
// All tests here verify that malformed/adversarial input returns errors, never panics.
// Issues: #1 #2 #3 #4 #9 #10

import (
	"encoding/binary"
	"testing"
)

// makeDRS53Sec builds a minimal Section 5 (DRS 5.3) byte slice with the given ng value.
// orderSD and nOctetsExtra must be in valid ranges (1-2 and 1-4 respectively) or parseDRS53
// will error before reaching the ng check.
func makeDRS53Sec(ng uint32, orderSD, nOctetsExtra byte) []byte {
	sec := make([]byte, 49) // minimum: 11+38
	binary.BigEndian.PutUint32(sec[0:4], 49)
	sec[4] = 5
	// sec[5:9] = N (don't care)
	binary.BigEndian.PutUint16(sec[9:11], 3) // template number = 3
	t := sec[11:]
	// t[36] = orderSD, t[37] = nOctetsExtra
	t[36] = orderSD
	t[37] = nOctetsExtra
	// t[20:24] = ng
	binary.BigEndian.PutUint32(t[20:24], ng)
	return sec
}

// makeSection3 builds a minimal Section 3 (GDT 3.30) byte slice.
func makeSection3(ni, nj uint32, scanMode byte) []byte {
	sec := make([]byte, 81) // minimum: 14+67
	binary.BigEndian.PutUint32(sec[0:4], 81)
	sec[4] = 3
	g := sec[14:]
	binary.BigEndian.PutUint32(g[16:20], ni)
	binary.BigEndian.PutUint32(g[20:24], nj)
	// lov = 262.5° → 262500000 µdeg
	binary.BigEndian.PutUint32(g[37:41], 262500000)
	// dx = dy = 3000000 mm (3000 m)
	binary.BigEndian.PutUint32(g[41:45], 3000000)
	binary.BigEndian.PutUint32(g[45:49], 3000000)
	// scanMode
	g[50] = scanMode
	// latin1 = latin2 = 38.5° = 38500000 µdeg
	binary.BigEndian.PutUint32(g[51:55], 38500000)
	binary.BigEndian.PutUint32(g[55:59], 38500000)
	return sec
}

// makeMinimalSec7 builds a Section 7 with enough bytes for the given order and nOctetsExtra,
// with no packed bit data (all bit widths assumed 0).
func makeMinimalSec7(order, m int) []byte {
	extraBytes := (order + 1) * m
	sec7 := make([]byte, 5+extraBytes)
	binary.BigEndian.PutUint32(sec7[0:4], uint32(5+extraBytes))
	sec7[4] = 7
	return sec7
}

// ---- parseDRS53 security tests ----

// TestParseDRS53NgTooLarge verifies that ng > maxNG returns an error, not an OOM alloc.
// Issue #1: Memory exhaustion via unbounded ng allocation.
func TestParseDRS53NgTooLarge(t *testing.T) {
	sec := makeDRS53Sec(0xFFFFFFFF, 2, 1)
	_, err := parseDRS53(sec)
	if err == nil {
		t.Fatal("parseDRS53 with ng=0xFFFFFFFF: expected error, got nil (would allocate ~96 GB)")
	}
}

// TestParseDRS53NgZero verifies that ng=0 returns an error.
// Issue #3: ng=0 causes lengths[-1] panic.
func TestParseDRS53NgZero(t *testing.T) {
	sec := makeDRS53Sec(0, 2, 1)
	_, err := parseDRS53(sec)
	if err == nil {
		t.Fatal("parseDRS53 with ng=0: expected error, got nil")
	}
}

// TestParseDRS53NgOne verifies that ng=1 is accepted as valid.
func TestParseDRS53NgOne(t *testing.T) {
	sec := makeDRS53Sec(1, 2, 1)
	p, err := parseDRS53(sec)
	if err != nil {
		t.Fatalf("parseDRS53 with ng=1: unexpected error: %v", err)
	}
	if p.NG != 1 {
		t.Errorf("NG: got %d, want 1", p.NG)
	}
}

// ---- unpackDRS53 security tests ----

// TestUnpackDRS53TotalOverflow verifies that total > maxTotal returns an error.
// Issue #2: total accumulation overflow + OOM.
func TestUnpackDRS53TotalOverflow(t *testing.T) {
	p := DRS53Params{
		OrderSpatialDiff: 2,
		NOctetsExtra:     1,
		NG:               1,
		LenLastGroup:     20_000_000, // total=20M > maxTotal=10M
		Nbits:            0,
		BitsGroupWidth:   0,
		BitsGroupLength:  0,
		LengthIncrement:  0,
	}
	sec7 := makeMinimalSec7(2, 1)
	_, err := unpackDRS53(sec7, p)
	if err == nil {
		t.Fatal("unpackDRS53 with total=20M: expected error, got nil (would allocate ~160 MB)")
	}
}

// TestUnpackDRS53Order2TotalZero verifies that order=2 with total=0 returns an error.
// Issue #4: second-order spatial differencing panics when total < 2.
func TestUnpackDRS53Order2TotalZero(t *testing.T) {
	p := DRS53Params{
		OrderSpatialDiff: 2,
		NOctetsExtra:     1,
		NG:               1,
		LenLastGroup:     0, // total=0
		Nbits:            0,
		BitsGroupWidth:   0,
		BitsGroupLength:  0,
	}
	sec7 := makeMinimalSec7(2, 1)
	_, err := unpackDRS53(sec7, p)
	if err == nil {
		t.Fatal("unpackDRS53 with order=2 total=0: expected error, got nil (would panic on undiff[0])")
	}
}

// TestUnpackDRS53Order2TotalOne verifies that order=2 with total=1 returns an error.
func TestUnpackDRS53Order2TotalOne(t *testing.T) {
	p := DRS53Params{
		OrderSpatialDiff: 2,
		NOctetsExtra:     1,
		NG:               1,
		LenLastGroup:     1, // total=1, need >= 2 for order-2
		Nbits:            0,
		BitsGroupWidth:   0,
		BitsGroupLength:  0,
	}
	sec7 := makeMinimalSec7(2, 1)
	_, err := unpackDRS53(sec7, p)
	if err == nil {
		t.Fatal("unpackDRS53 with order=2 total=1: expected error, got nil (would panic on undiff[1])")
	}
}

// TestUnpackDRS53Order1TotalZero verifies that order=1 with total=0 returns an error.
func TestUnpackDRS53Order1TotalZero(t *testing.T) {
	p := DRS53Params{
		OrderSpatialDiff: 1,
		NOctetsExtra:     1,
		NG:               1,
		LenLastGroup:     0,
		Nbits:            0,
		BitsGroupWidth:   0,
		BitsGroupLength:  0,
	}
	sec7 := makeMinimalSec7(1, 1)
	_, err := unpackDRS53(sec7, p)
	if err == nil {
		t.Fatal("unpackDRS53 with order=1 total=0: expected error, got nil (would panic on undiff[0])")
	}
}

// ---- sectionAt security tests ----

// TestSectionAtLargeSLen verifies that a section length that would overflow the buffer returns an error.
// Issue #10: integer overflow in off+int(sLen).
func TestSectionAtLargeSLen(t *testing.T) {
	buf := make([]byte, 10)
	binary.BigEndian.PutUint32(buf[0:4], 100) // sLen=100 > len(buf)=10
	buf[4] = 5
	_, _, _, _, err := sectionAt(buf, 0)
	if err == nil {
		t.Fatal("sectionAt with sLen > buf: expected error, got nil")
	}
}

// TestSectionAtMaxUint32SLen verifies that sLen=0xFFFFFFFF is caught safely.
// This tests the integer overflow case — old code: off+int(0xFFFFFFFF) could wrap to negative.
func TestSectionAtMaxUint32SLen(t *testing.T) {
	buf := make([]byte, 10)
	binary.BigEndian.PutUint32(buf[0:4], 0xFFFFFFFF)
	buf[4] = 5
	_, _, _, _, err := sectionAt(buf, 0)
	if err == nil {
		t.Fatal("sectionAt with sLen=0xFFFFFFFF: expected error, got nil")
	}
}

// TestSectionAtValidSection verifies a normal valid section is parsed correctly.
func TestSectionAtValidSection(t *testing.T) {
	buf := make([]byte, 10)
	binary.BigEndian.PutUint32(buf[0:4], 10) // sLen=10, fits exactly
	buf[4] = 3                               // section number
	sLen, sNum, sec, next, err := sectionAt(buf, 0)
	if err != nil {
		t.Fatalf("sectionAt: unexpected error: %v", err)
	}
	if sLen != 10 {
		t.Errorf("sLen: got %d, want 10", sLen)
	}
	if sNum != 3 {
		t.Errorf("sNum: got %d, want 3", sNum)
	}
	if len(sec) != 10 {
		t.Errorf("sec len: got %d, want 10", len(sec))
	}
	if next != 10 {
		t.Errorf("next: got %d, want 10", next)
	}
}

// ---- parseSection3HRRR security tests ----

// TestSection3UnsupportedScanMode verifies that scanMode != 0x40 returns an error.
// Issue #9: ScanMode parsed but never validated — wrong data returned silently.
func TestSection3UnsupportedScanMode(t *testing.T) {
	for _, scanMode := range []byte{0x00, 0x80, 0xC0, 0x01} {
		sec := makeSection3(100, 100, scanMode)
		_, err := parseSection3HRRR(sec)
		if err == nil {
			t.Errorf("parseSection3HRRR with scanMode=0x%02X: expected error, got nil", scanMode)
		}
	}
}

// TestSection3ValidScanMode verifies that scanMode=0x40 is accepted.
func TestSection3ValidScanMode(t *testing.T) {
	sec := makeSection3(1799, 1059, 0x40)
	s3, err := parseSection3HRRR(sec)
	if err != nil {
		t.Fatalf("parseSection3HRRR with scanMode=0x40: unexpected error: %v", err)
	}
	if s3.Grid.ScanMode != 0x40 {
		t.Errorf("ScanMode: got 0x%02X, want 0x40", s3.Grid.ScanMode)
	}
}

// TestSection3InvalidGridDims verifies that unreasonable Ni/Nj values return an error.
func TestSection3InvalidGridDims(t *testing.T) {
	tests := []struct {
		ni, nj uint32
		name   string
	}{
		{0, 100, "ni=0"},
		{100, 0, "nj=0"},
		{40000, 100, "ni too large"},
		{100, 40000, "nj too large"},
	}
	for _, tc := range tests {
		sec := makeSection3(tc.ni, tc.nj, 0x40)
		_, err := parseSection3HRRR(sec)
		if err == nil {
			t.Errorf("parseSection3HRRR %s: expected error, got nil", tc.name)
		}
	}
}

// ---- decodeScaleFactor tests ----

// TestDecodeScaleFactor verifies GRIB2 sign-magnitude scale factor decoding.
// Issue #15: unit tests for pure functions.
func TestDecodeScaleFactor(t *testing.T) {
	tests := []struct {
		raw  uint16
		want int
	}{
		{0x0000, 0},      // zero
		{0x0001, 1},      // +1
		{0x8001, -1},     // -1 (sign bit set, magnitude 1)
		{0x7FFF, 32767},  // max positive
		{0xFFFF, -32767}, // max negative
		{0x8000, 0},      // -0 (sign bit, magnitude 0) → 0
	}
	for _, tc := range tests {
		got := decodeScaleFactor(tc.raw)
		if got != tc.want {
			t.Errorf("decodeScaleFactor(0x%04X) = %d, want %d", tc.raw, got, tc.want)
		}
	}
}

// readUintOctets and readSignMagOctets table tests live in drs53_test.go to avoid redeclaration.

// ---- DecodeMessage error paths (no network) ----

func TestDecodeMessageEmptyReturnsError(t *testing.T) {
	_, err := DecodeMessage([]byte{})
	if err == nil {
		t.Error("DecodeMessage(empty): expected error, got nil")
	}
}

func TestDecodeMessageBadMagicReturnsError(t *testing.T) {
	buf := make([]byte, 16)
	copy(buf, "NOPE")
	_, err := DecodeMessage(buf)
	if err == nil {
		t.Error("DecodeMessage(bad magic): expected error, got nil")
	}
}

func TestDecodeMessageNoSection3ReturnsError(t *testing.T) {
	// Valid section 0 (16 bytes) followed by immediate end marker "7777".
	buf := make([]byte, 20)
	copy(buf[0:4], "GRIB")
	buf[7] = 2
	// TotalLength covers all 20 bytes
	buf[8] = 0
	buf[9] = 0
	buf[10] = 0
	buf[11] = 0
	buf[12] = 0
	buf[13] = 0
	buf[14] = 0
	buf[15] = 20
	copy(buf[16:20], "7777")

	_, err := DecodeMessage(buf)
	if err == nil {
		t.Error("DecodeMessage with no section 3: expected error, got nil")
	}
}

// TestDecodeMessageUnsupportedDRSTemplateReturnsError verifies that an unknown DRS template
// returns an error. Template 0 is now supported via DRS 5.0; the error comes from parseDRS0
// receiving a too-short section (11 bytes instead of 21).
func TestDecodeMessageUnsupportedDRSTemplateReturnsError(t *testing.T) {
	// Build a minimal message: sec0 + sec1 + sec3 (valid) + sec5 (template 0, too short) + "7777"
	// This tests the error path in DecodeMessage without needing real GRIB data.

	// Section 0: 16 bytes
	sec0 := make([]byte, 16)
	copy(sec0[0:4], "GRIB")
	sec0[7] = 2

	// Section 1: minimal 21-byte identification section
	sec1 := make([]byte, 21)
	sec1[0] = 0
	sec1[1] = 0
	sec1[2] = 0
	sec1[3] = 21
	sec1[4] = 1

	// Section 3: 14 + 67 = 81 bytes minimum, section number = 3
	sec3 := make([]byte, 81)
	sec3[0] = 0
	sec3[1] = 0
	sec3[2] = 0
	sec3[3] = 81
	sec3[4] = 3

	// Section 5 with template number = 0 but only 11 bytes (parseDRS0 needs 21)
	sec5 := make([]byte, 11)
	sec5[0] = 0
	sec5[1] = 0
	sec5[2] = 0
	sec5[3] = 11
	sec5[4] = 5
	// sec5[9:11] = template number 0 (big-endian)
	sec5[9] = 0
	sec5[10] = 0

	end := []byte("7777")

	var buf []byte
	buf = append(buf, sec0...)
	buf = append(buf, sec1...)
	buf = append(buf, sec3...)
	buf = append(buf, sec5...)
	buf = append(buf, end...)

	_, err := DecodeMessage(buf)
	if err == nil {
		t.Error("DecodeMessage with too-short DRS 5.0 section: expected error, got nil")
	}
}

// TestDecodeMessageUnsupportedBitmapIndicatorReturnsError verifies that a Section 6
// with an unrecognised bitmap indicator (not 0 or 255) returns an error.
func TestDecodeMessageUnsupportedBitmapIndicatorReturnsError(t *testing.T) {
	sec0 := make([]byte, 16)
	copy(sec0[0:4], "GRIB")
	sec0[7] = 2

	// Section 6: flag=1 (predefined bitmap, not supported)
	sec6 := []byte{0x00, 0x00, 0x00, 0x06, 0x06, 0x01}

	end := []byte("7777")

	var buf []byte
	buf = append(buf, sec0...)
	buf = append(buf, sec6...)
	buf = append(buf, end...)

	_, err := DecodeMessage(buf)
	if err == nil {
		t.Error("DecodeMessage with bitmap indicator=1: expected error, got nil")
	}
}
