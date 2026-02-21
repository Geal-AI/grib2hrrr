package grib2hrrr

import (
	"encoding/binary"
	"fmt"
)

// bitReader reads unsigned integers of arbitrary bit width from a byte slice.
// Bits are consumed MSB-first within each byte (big-endian bit order).
type bitReader struct {
	buf []byte
	pos int // current bit position
}

func newBitReader(b []byte) *bitReader { return &bitReader{buf: b} }

// read reads n bits (0 ≤ n ≤ 64) and returns them as a uint64.
// Issue #5: mustRead has been removed — library code must never panic on untrusted
// input. All callers use read() with proper error handling.
// Issue #19: byte-aligned reads of 8/16/32/64 bits use binary.BigEndian for speed.
func (r *bitReader) read(n int) (uint64, error) {
	if n == 0 {
		return 0, nil
	}
	end := r.pos + n
	if end > len(r.buf)*8 {
		return 0, fmt.Errorf("bitReader: read %d bits at pos %d overflows buffer (%d bytes)",
			n, r.pos, len(r.buf))
	}
	// Fast path: byte-aligned reads of exact byte widths.
	if r.pos%8 == 0 {
		off := r.pos / 8
		switch n {
		case 8:
			r.pos = end
			return uint64(r.buf[off]), nil
		case 16:
			r.pos = end
			return uint64(binary.BigEndian.Uint16(r.buf[off:])), nil
		case 32:
			r.pos = end
			return uint64(binary.BigEndian.Uint32(r.buf[off:])), nil
		case 64:
			r.pos = end
			return binary.BigEndian.Uint64(r.buf[off:]), nil
		}
	}
	// Slow path: bit-by-bit for non-aligned or non-standard widths.
	var v uint64
	for i := 0; i < n; i++ {
		byteIdx := (r.pos + i) / 8
		bitIdx := 7 - ((r.pos + i) % 8) // MSB first within byte
		bit := (r.buf[byteIdx] >> bitIdx) & 1
		v = (v << 1) | uint64(bit)
	}
	r.pos = end
	return v, nil
}

// align advances pos to the next byte boundary.
func (r *bitReader) align() {
	if r.pos%8 != 0 {
		r.pos += 8 - (r.pos % 8)
	}
}

// bytePos returns the current byte position (must be byte-aligned).
func (r *bitReader) bytePos() int { return r.pos / 8 }
