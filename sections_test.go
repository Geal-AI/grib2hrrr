package grib2hrrr

import (
	"testing"
)

// ---------------------------------------------------------------------------
// decodeScaleFactor
// ---------------------------------------------------------------------------

func TestDecodeScaleFactorPositive(t *testing.T) {
	cases := []struct {
		raw  uint16
		want int
	}{
		{0x0000, 0},
		{0x0001, 1},
		{0x7FFF, 32767},
		{0x000A, 10},
	}
	for _, tc := range cases {
		got := decodeScaleFactor(tc.raw)
		if got != tc.want {
			t.Errorf("decodeScaleFactor(0x%04X): got %d, want %d", tc.raw, got, tc.want)
		}
	}
}

func TestDecodeScaleFactorNegative(t *testing.T) {
	cases := []struct {
		raw  uint16
		want int
	}{
		{0x8001, -1},
		{0x8000, 0}, // sign bit set, magnitude 0 → −0 = 0
		{0xFFFF, -32767},
		{0x800A, -10},
	}
	for _, tc := range cases {
		got := decodeScaleFactor(tc.raw)
		if got != tc.want {
			t.Errorf("decodeScaleFactor(0x%04X): got %d, want %d", tc.raw, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// parseSection0
// ---------------------------------------------------------------------------

func TestParseSection0ValidHeader(t *testing.T) {
	buf := make([]byte, 16)
	copy(buf[0:4], "GRIB")
	buf[6] = 0 // discipline: meteorological
	buf[7] = 2 // edition 2
	// total length = 0x0000_0000_0001_0000 = 65536
	buf[8] = 0
	buf[9] = 0
	buf[10] = 0
	buf[11] = 0
	buf[12] = 0
	buf[13] = 1
	buf[14] = 0
	buf[15] = 0

	s, err := parseSection0(buf)
	if err != nil {
		t.Fatalf("parseSection0 error: %v", err)
	}
	if s.Discipline != 0 {
		t.Errorf("Discipline: got %d, want 0", s.Discipline)
	}
	if s.Edition != 2 {
		t.Errorf("Edition: got %d, want 2", s.Edition)
	}
	if s.TotalLength != 65536 {
		t.Errorf("TotalLength: got %d, want 65536", s.TotalLength)
	}
}

func TestParseSection0TooShortReturnsError(t *testing.T) {
	_, err := parseSection0([]byte("GRIB"))
	if err == nil {
		t.Error("parseSection0 on 4-byte input: expected error, got nil")
	}
}

func TestParseSection0BadMagicReturnsError(t *testing.T) {
	buf := make([]byte, 16)
	copy(buf[0:4], "NOTG")
	_, err := parseSection0(buf)
	if err == nil {
		t.Error("parseSection0 with wrong magic: expected error, got nil")
	}
}

func TestParseSection0EmptyReturnsError(t *testing.T) {
	_, err := parseSection0([]byte{})
	if err == nil {
		t.Error("parseSection0 on empty input: expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// sectionAt
// ---------------------------------------------------------------------------

func TestSectionAtNormalSection(t *testing.T) {
	// Build a minimal section: 4-byte big-endian length (9), section number 1,
	// then 4 bytes of payload.
	buf := []byte{
		0x00, 0x00, 0x00, 0x09, // length = 9
		0x01,                   // section number = 1
		0xAA, 0xBB, 0xCC, 0xDD, // 4 bytes payload
	}
	sLen, sNum, sec, next, err := sectionAt(buf, 0)
	if err != nil {
		t.Fatalf("sectionAt error: %v", err)
	}
	if sLen != 9 {
		t.Errorf("sLen: got %d, want 9", sLen)
	}
	if sNum != 1 {
		t.Errorf("sNum: got %d, want 1", sNum)
	}
	if len(sec) != 9 {
		t.Errorf("sec length: got %d, want 9", len(sec))
	}
	if next != 9 {
		t.Errorf("next: got %d, want 9", next)
	}
}

func TestSectionAtEndMarker(t *testing.T) {
	buf := []byte("7777")
	sLen, sNum, _, next, err := sectionAt(buf, 0)
	if err != nil {
		t.Fatalf("sectionAt '7777' error: %v", err)
	}
	if sLen != 4 {
		t.Errorf("end marker sLen: got %d, want 4", sLen)
	}
	if sNum != 8 {
		t.Errorf("end marker sNum: got %d, want 8", sNum)
	}
	if next != 4 {
		t.Errorf("end marker next: got %d, want 4", next)
	}
}

func TestSectionAtOutOfBoundsHeaderReturnsError(t *testing.T) {
	buf := []byte{0x00, 0x00} // fewer than 5 bytes
	_, _, _, _, err := sectionAt(buf, 0)
	if err == nil {
		t.Error("sectionAt on truncated header: expected error, got nil")
	}
}

func TestSectionAtLengthOverflowsBufferReturnsError(t *testing.T) {
	// Claim section is 100 bytes long, but buffer is only 9 bytes.
	buf := []byte{
		0x00, 0x00, 0x00, 0x64, // length = 100
		0x03, // section 3
		0x00, 0x00, 0x00, 0x00,
	}
	_, _, _, _, err := sectionAt(buf, 0)
	if err == nil {
		t.Error("sectionAt with overflow length: expected error, got nil")
	}
}

func TestSectionAtNonZeroOffset(t *testing.T) {
	// Place a valid section at byte offset 4.
	prefix := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	section := []byte{
		0x00, 0x00, 0x00, 0x05, // length = 5
		0x07, // section 7
	}
	buf := append(prefix, section...)
	_, sNum, _, _, err := sectionAt(buf, 4)
	if err != nil {
		t.Fatalf("sectionAt at offset 4: %v", err)
	}
	if sNum != 7 {
		t.Errorf("sNum at offset 4: got %d, want 7", sNum)
	}
}
