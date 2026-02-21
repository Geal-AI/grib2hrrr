package grib2hrrr

import (
	"testing"
)

// TestBitReaderReadZeroBits verifies that reading 0 bits returns 0 without advancing position.
func TestBitReaderReadZeroBits(t *testing.T) {
	r := newBitReader([]byte{0xFF})
	v, err := r.read(0)
	if err != nil {
		t.Fatalf("read(0) error: %v", err)
	}
	if v != 0 {
		t.Errorf("read(0): got %d, want 0", v)
	}
	if r.pos != 0 {
		t.Errorf("read(0) advanced pos to %d, want 0", r.pos)
	}
}

// TestBitReaderReadSingleByte verifies reading all 8 bits of a single byte.
func TestBitReaderReadSingleByte(t *testing.T) {
	r := newBitReader([]byte{0b10110100})
	v, err := r.read(8)
	if err != nil {
		t.Fatalf("read(8) error: %v", err)
	}
	if v != 0b10110100 {
		t.Errorf("read(8): got %08b, want 10110100", v)
	}
}

// TestBitReaderReadMSBFirst verifies that bits are consumed MSB-first within each byte.
func TestBitReaderReadMSBFirst(t *testing.T) {
	// 0b10000000: only the MSB is set
	r := newBitReader([]byte{0b10000000})
	v, err := r.read(1)
	if err != nil {
		t.Fatalf("read(1) error: %v", err)
	}
	if v != 1 {
		t.Errorf("read(1) from 0x80: got %d, want 1", v)
	}
	v, err = r.read(1)
	if err != nil {
		t.Fatalf("second read(1) error: %v", err)
	}
	if v != 0 {
		t.Errorf("second read(1) from 0x80: got %d, want 0", v)
	}
}

// TestBitReaderReadCrossesBytes verifies reading spans two bytes correctly.
func TestBitReaderReadCrossesBytes(t *testing.T) {
	// bytes: 0b00000001 0b10000000
	// bits: 0000 0001 | 1000 0000
	// reading 10 bits starting at bit 0: 0000000110 = 6
	r := newBitReader([]byte{0x01, 0x80})
	v, err := r.read(10)
	if err != nil {
		t.Fatalf("read(10) error: %v", err)
	}
	if v != 0b0000000110 {
		t.Errorf("read(10): got %010b (%d), want 0000000110 (6)", v, v)
	}
}

// TestBitReaderSequentialReads verifies that multiple sequential reads accumulate position correctly.
func TestBitReaderSequentialReads(t *testing.T) {
	// 0xAB = 0b10101011
	r := newBitReader([]byte{0xAB})
	cases := []struct {
		bits uint
		want uint64
	}{
		{1, 1}, // MSB: 1
		{1, 0}, // next: 0
		{1, 1}, // next: 1
		{1, 0}, // next: 0
		{1, 1}, // next: 1
		{1, 0}, // next: 0
		{1, 1}, // next: 1
		{1, 1}, // LSB: 1
	}
	for i, tc := range cases {
		v, err := r.read(int(tc.bits))
		if err != nil {
			t.Fatalf("step %d read(%d) error: %v", i, tc.bits, err)
		}
		if v != tc.want {
			t.Errorf("step %d read(%d): got %d, want %d", i, tc.bits, v, tc.want)
		}
	}
}

// TestBitReaderOverflowReturnsError verifies that reading past the buffer returns an error.
func TestBitReaderOverflowReturnsError(t *testing.T) {
	r := newBitReader([]byte{0xFF})
	_, err := r.read(9) // 9 bits from a 1-byte (8-bit) buffer
	if err == nil {
		t.Error("read(9) from 1-byte buffer: expected error, got nil")
	}
}

// TestBitReaderOverflowEmptyBuffer verifies error on read from empty buffer.
func TestBitReaderOverflowEmptyBuffer(t *testing.T) {
	r := newBitReader([]byte{})
	_, err := r.read(1)
	if err == nil {
		t.Error("read(1) from empty buffer: expected error, got nil")
	}
}

// TestBitReaderNoMustRead documents that mustRead has been removed from the library.
// Library code must never panic on untrusted input — use read() with error return instead.
// Issue #5: mustRead was dead code that panics on corrupt input.

// TestBitReaderAlignOnBoundary verifies align is a no-op when already byte-aligned.
func TestBitReaderAlignOnBoundary(t *testing.T) {
	r := newBitReader([]byte{0xFF, 0x00})
	r.read(8) // advance to byte boundary
	before := r.pos
	r.align()
	if r.pos != before {
		t.Errorf("align() on boundary moved pos from %d to %d", before, r.pos)
	}
}

// TestBitReaderAlignMidByte verifies align pads to the next byte boundary.
func TestBitReaderAlignMidByte(t *testing.T) {
	r := newBitReader([]byte{0xFF, 0x00})
	r.read(3) // pos = 3 (mid-byte)
	r.align()
	if r.pos != 8 {
		t.Errorf("align() after 3 bits: pos=%d, want 8", r.pos)
	}
}

// TestBitReaderBytePos verifies bytePos returns the correct byte offset.
func TestBitReaderBytePos(t *testing.T) {
	r := newBitReader([]byte{0xFF, 0x00, 0xAB})
	r.read(8)  // consume first byte
	r.read(8)  // consume second byte
	if r.bytePos() != 2 {
		t.Errorf("bytePos(): got %d, want 2", r.bytePos())
	}
}

// TestBitReaderRead64Bits verifies reading a full 64-bit value across 8 bytes.
func TestBitReaderRead64Bits(t *testing.T) {
	buf := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	r := newBitReader(buf)
	v, err := r.read(64)
	if err != nil {
		t.Fatalf("read(64) error: %v", err)
	}
	want := uint64(0x0102030405060708)
	if v != want {
		t.Errorf("read(64): got 0x%016X, want 0x%016X", v, want)
	}
}

// TestBitReaderReadKnownPattern verifies a multi-bit pattern used in GRIB2 group reference encoding.
// Encoding 3 values of 5 bits each in two bytes: [10110 01100 1xxxxx] packed MSB-first.
// 0b10110011 0b00100000 = 0xB3 0x20
// bits: 1 0 1 1 0 | 0 1 1 0 0 | 1 0 0 0 0 0 (padded)
// value[0] = 0b10110 = 22, value[1] = 0b01100 = 12, value[2] = 0b10000 = 16
func TestBitReaderReadKnownPattern(t *testing.T) {
	r := newBitReader([]byte{0xB3, 0x20})
	cases := []struct {
		bits int
		want uint64
	}{
		{5, 22}, // 10110
		{5, 12}, // 01100
		{5, 16}, // 10000 (upper bits of second byte)
	}
	for i, tc := range cases {
		v, err := r.read(tc.bits)
		if err != nil {
			t.Fatalf("case %d read(%d) error: %v", i, tc.bits, err)
		}
		if v != tc.want {
			t.Errorf("case %d read(%d): got %d, want %d", i, tc.bits, v, tc.want)
		}
	}
}
