package grib2hrrr

import (
	"encoding/binary"
	"fmt"
	"math"
)

// unpackDRS53 decodes a DRS Template 5.3 (complex packing + spatial differencing)
// Section 7. sec7 is the raw section 7 bytes (including the 5-byte section header).
// p is DRS53Params decoded from Section 5.
func unpackDRS53(sec7 []byte, p DRS53Params) ([]float64, error) {
	if len(sec7) < 5 {
		return nil, fmt.Errorf("drs53: section 7 too short")
	}
	data := sec7[5:] // skip 4-byte length + 1-byte section number

	m := p.NOctetsExtra
	order := p.OrderSpatialDiff
	if order < 1 || order > 2 {
		return nil, fmt.Errorf("drs53: unsupported spatial differencing order %d", order)
	}
	if m < 1 || m > 4 {
		return nil, fmt.Errorf("drs53: unsupported extra descriptor octets %d", m)
	}

	// --- Step 1: Read extra descriptors (initial values + minimum bias) ---
	extraBytes := (order + 1) * m
	if len(data) < extraBytes {
		return nil, fmt.Errorf("drs53: data too short for extra descriptors (%d < %d)", len(data), extraBytes)
	}

	initVals := make([]int64, order)
	for i := 0; i < order; i++ {
		initVals[i] = readSignMagOctets(data[i*m : i*m+m])
	}
	yMin := readSignMagOctets(data[order*m : order*m+m])

	// Packed bits begin after extra descriptors
	br := newBitReader(data[extraBytes:])

	// --- Step 2: Read group reference values (NG × nBits bits) ---
	ng := p.NG
	nBits := p.Nbits
	grefs := make([]int64, ng)
	for i := 0; i < ng; i++ {
		v, err := br.read(nBits)
		if err != nil {
			return nil, fmt.Errorf("drs53: reading gref[%d]: %w", i, err)
		}
		grefs[i] = int64(v)
	}
	br.align() // WMO Note (6): group reference values must end on a byte boundary

	// --- Step 3: Read group widths (NG × bitsGroupWidth bits) ---
	// WMO Template 7.3 Note (6): group widths must end on a byte boundary.
	widths := make([]int, ng)
	for i := 0; i < ng; i++ {
		v, err := br.read(p.BitsGroupWidth)
		if err != nil {
			return nil, fmt.Errorf("drs53: reading width[%d]: %w", i, err)
		}
		widths[i] = p.RefGroupWidth + int(v)
	}
	br.align() // pad to byte boundary after widths

	// --- Step 4: Read group lengths (NG × bitsGroupLength bits) ---
	// WMO Template 7.3 Note (6): group lengths must also end on a byte boundary.
	lengths := make([]int, ng)
	for i := 0; i < ng-1; i++ {
		v, err := br.read(p.BitsGroupLength)
		if err != nil {
			return nil, fmt.Errorf("drs53: reading length[%d]: %w", i, err)
		}
		lengths[i] = int(v)*int(p.LengthIncrement) + int(p.RefGroupLength)
	}
	// Last group uses the true length from the DRS header
	{
		_, err := br.read(p.BitsGroupLength) // still consume the bits
		if err != nil {
			return nil, fmt.Errorf("drs53: reading length[last]: %w", err)
		}
	}
	lengths[ng-1] = int(p.LenLastGroup)
	br.align() // pad to byte boundary after lengths

	// --- Step 5: Read grouped data values ---
	total := 0
	for _, l := range lengths {
		total += l
	}
	packed := make([]int64, 0, total)

	for g := 0; g < ng; g++ {
		gref := grefs[g]
		w := widths[g]
		l := lengths[g]
		for k := 0; k < l; k++ {
			if w == 0 {
				packed = append(packed, gref)
			} else {
				v, err := br.read(w)
				if err != nil {
					return nil, fmt.Errorf("drs53: reading group %d val %d: %w", g, k, err)
				}
				packed = append(packed, gref+int64(v))
			}
		}
	}

	if len(packed) != total {
		return nil, fmt.Errorf("drs53: expected %d values, got %d", total, len(packed))
	}

	// --- Step 6: Add minimum bias (yMin) to restore differenced values ---
	z := make([]int64, total)
	for i := range packed {
		z[i] = packed[i] + yMin
	}

	// --- Step 7: Undo spatial differencing ---
	undiff := make([]int64, total)
	switch order {
	case 1:
		undiff[0] = initVals[0]
		for i := 1; i < total; i++ {
			undiff[i] = z[i] + undiff[i-1]
		}
	case 2:
		undiff[0] = initVals[0]
		undiff[1] = initVals[1]
		for i := 2; i < total; i++ {
			undiff[i] = z[i] + 2*undiff[i-1] - undiff[i-2]
		}
	}

	// --- Step 8: Apply scale formula: Y = (R + 2^E * X) / 10^D ---
	R := p.ReferenceValue
	scaleE := math.Pow(2, float64(p.BinaryScaleFactor))
	scaleD := math.Pow(10, float64(p.DecimalScaleFactor))

	result := make([]float64, total)
	for i, x := range undiff {
		result[i] = (R + scaleE*float64(x)) / scaleD
	}
	return result, nil
}

// readSignMagOctets reads an m-byte sign-magnitude integer (GRIB2 convention).
// The most significant bit is the sign bit (1 = negative).
func readSignMagOctets(b []byte) int64 {
	if len(b) == 0 {
		return 0
	}
	raw := readUintOctets(b)
	bits := uint64(len(b)) * 8
	signBit := uint64(1) << (bits - 1)
	if raw&signBit != 0 {
		return -int64(raw &^ signBit)
	}
	return int64(raw)
}

// readUintOctets reads a big-endian unsigned integer from 1–4 bytes.
func readUintOctets(b []byte) uint64 {
	switch len(b) {
	case 1:
		return uint64(b[0])
	case 2:
		return uint64(binary.BigEndian.Uint16(b))
	case 3:
		return uint64(b[0])<<16 | uint64(b[1])<<8 | uint64(b[2])
	case 4:
		return uint64(binary.BigEndian.Uint32(b))
	default:
		var v uint64
		for _, byt := range b {
			v = (v << 8) | uint64(byt)
		}
		return v
	}
}
