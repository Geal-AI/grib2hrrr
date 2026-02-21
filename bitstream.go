package grib2hrrr

import "fmt"

// bitReader reads unsigned integers of arbitrary bit width from a byte slice.
// Bits are consumed MSB-first within each byte (big-endian bit order).
type bitReader struct {
	buf  []byte
	pos  int // current bit position
}

func newBitReader(b []byte) *bitReader { return &bitReader{buf: b} }

// read reads n bits (0 ≤ n ≤ 64) and returns them as a uint64.
func (r *bitReader) read(n int) (uint64, error) {
	if n == 0 {
		return 0, nil
	}
	end := r.pos + n
	if end > len(r.buf)*8 {
		return 0, fmt.Errorf("bitReader: read %d bits at pos %d overflows buffer (%d bytes)",
			n, r.pos, len(r.buf))
	}
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

// mustRead panics on error; used where errors indicate a corrupt file.
func (r *bitReader) mustRead(n int) uint64 {
	v, err := r.read(n)
	if err != nil {
		panic(err)
	}
	return v
}

// align advances pos to the next byte boundary.
func (r *bitReader) align() {
	if r.pos%8 != 0 {
		r.pos += 8 - (r.pos % 8)
	}
}

// bytePos returns the current byte position (must be byte-aligned).
func (r *bitReader) bytePos() int { return r.pos / 8 }
