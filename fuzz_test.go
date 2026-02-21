package grib2hrrr

import (
	"testing"
)

// FuzzDecodeMessage feeds arbitrary byte slices to DecodeMessage.
// The invariant is that it must never panic — only return an error or a valid Field.
// Run with: go test -fuzz=FuzzDecodeMessage -fuzztime=60s ./...
func FuzzDecodeMessage(f *testing.F) {
	// Seed corpus: known-good section 0 prefix and common malformed inputs.
	seeds := [][]byte{
		// Valid GRIB magic, too short to decode
		[]byte("GRIB\x00\x00\x00\x02\x00\x00\x00\x00\x00\x00\x00\x10"),
		// Wrong magic
		[]byte("NOTGRIB"),
		// Empty
		{},
		// Just the magic
		[]byte("GRIB"),
		// Valid sec0 + end marker
		func() []byte {
			b := make([]byte, 20)
			copy(b[0:4], "GRIB")
			b[7] = 2
			b[15] = 20
			copy(b[16:], "7777")
			return b
		}(),
		// Random short bytes
		{0x00, 0x01, 0x02, 0x03},
		{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
			0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must not panic.
		_, _ = DecodeMessage(data)
	})
}

// FuzzUnpackDRS53 feeds arbitrary section-7 bytes with fixed plausible params.
// The invariant: no panic, only error or valid []float64.
// Run with: go test -fuzz=FuzzUnpackDRS53 -fuzztime=60s ./...
func FuzzUnpackDRS53(f *testing.F) {
	// Seed with a valid minimal sec7 (5-byte header + small payload)
	seeds := [][]byte{
		{0x00, 0x00, 0x00, 0x00, 0x07, 0x0A, 0x00, 0x00, 0x00, 0x00, 0x01, 0x20},
		{0x00, 0x00, 0x00, 0x00, 0x07},
		{},
		{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
		make([]byte, 64),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	// Use params representative of real HRRR data but small enough to be safe.
	p := DRS53Params{
		ReferenceValue:     0,
		BinaryScaleFactor:  0,
		DecimalScaleFactor: 0,
		Nbits:              4,
		NG:                 1,
		RefGroupWidth:      4,
		BitsGroupWidth:     4,
		RefGroupLength:     0,
		LengthIncrement:    1,
		LenLastGroup:       3,
		BitsGroupLength:    4,
		OrderSpatialDiff:   1,
		NOctetsExtra:       1,
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must not panic.
		_, _ = unpackDRS53(data, p)
	})
}

// FuzzBitReaderRead verifies that the bitReader never panics regardless of input.
// Run with: go test -fuzz=FuzzBitReaderRead -fuzztime=30s ./...
func FuzzBitReaderRead(f *testing.F) {
	f.Add([]byte{0xFF, 0x00, 0xAB, 0xCD}, 7)
	f.Add([]byte{}, 0)
	f.Add([]byte{0x00}, 8)
	f.Add([]byte{0x00}, 1)

	f.Fuzz(func(t *testing.T, data []byte, nBits int) {
		if nBits < 0 || nBits > 64 {
			return // clamp to valid range
		}
		r := newBitReader(data)
		// Must not panic — may return error.
		_, _ = r.read(nBits)
	})
}
