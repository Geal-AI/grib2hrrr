package grib2hrrr

import (
	"fmt"
	"math"
)

// applyBitmap expands packed values (one per set bitmap bit) to a full
// totalPoints grid. Positions where the bitmap bit is 0 are filled with NaN.
//
// GRIB2 bitmaps are MSB-first: bit 7 of byte 0 is grid point 0,
// bit 6 of byte 0 is grid point 1, and so on.
func applyBitmap(vals []float64, bitmap []byte, totalPoints int) ([]float64, error) {
	setBits := countSetBits(bitmap, totalPoints)
	if setBits != len(vals) {
		return nil, fmt.Errorf("bitmap: %d set bits but %d packed values", setBits, len(vals))
	}

	result := make([]float64, totalPoints)
	vi := 0
	for i := 0; i < totalPoints; i++ {
		if bitmapBit(bitmap, i) {
			result[i] = vals[vi]
			vi++
		} else {
			result[i] = math.NaN()
		}
	}
	return result, nil
}

// bitmapBit reports whether grid point i has data (bit set) in the MSB-first bitmap.
func bitmapBit(bitmap []byte, i int) bool {
	byteIdx := i / 8
	if byteIdx >= len(bitmap) {
		return false
	}
	return (bitmap[byteIdx]>>uint(7-(i%8)))&1 == 1
}

// countSetBits counts the number of set bits for the first totalPoints positions.
func countSetBits(bitmap []byte, totalPoints int) int {
	n := 0
	for i := 0; i < totalPoints; i++ {
		if bitmapBit(bitmap, i) {
			n++
		}
	}
	return n
}
