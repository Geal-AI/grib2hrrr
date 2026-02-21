package grib2hrrr

import (
	"encoding/binary"
	"fmt"
	"math"
)

// DRS0Params holds parameters from DRS Template 5.0 (grid point, simple packing).
type DRS0Params struct {
	ReferenceValue     float64
	BinaryScaleFactor  int
	DecimalScaleFactor int
	Nbits              int
	TypeOfValue        byte
	N                  int // number of data points from sec[5:9]
}

// parseDRS0 decodes Section 5 with DRS Template 5.0.
func parseDRS0(sec []byte) (DRS0Params, error) {
	// sec[0:4]=len, sec[4]=5, sec[5:9]=N, sec[9:11]=template_num, sec[11:]=template data
	if len(sec) < 11+10 {
		return DRS0Params{}, fmt.Errorf("section 5 DRS 5.0: too short (%d bytes)", len(sec))
	}

	nRaw := binary.BigEndian.Uint32(sec[5:9])
	if nRaw > uint32(maxTotal) {
		return DRS0Params{}, fmt.Errorf("section 5: N=%d exceeds maximum %d", nRaw, maxTotal)
	}

	t := sec[11:] // template data
	refBits := binary.BigEndian.Uint32(t[0:4])
	R := math.Float32frombits(refBits)
	E := decodeScaleFactor(binary.BigEndian.Uint16(t[4:6]))
	D := decodeScaleFactor(binary.BigEndian.Uint16(t[6:8]))
	nBits := int(t[8])
	typeVal := t[9]

	if nBits > maxBitWidth {
		return DRS0Params{}, fmt.Errorf("section 5: Nbits=%d exceeds %d", nBits, maxBitWidth)
	}

	return DRS0Params{
		ReferenceValue:     float64(R),
		BinaryScaleFactor:  E,
		DecimalScaleFactor: D,
		Nbits:              nBits,
		TypeOfValue:        typeVal,
		N:                  int(nRaw),
	}, nil
}

// unpackDRS0 decodes a DRS Template 5.0 (simple packing) Section 7.
// Values are N consecutive nBits-wide unsigned integers packed MSB-first.
// Unpacking formula: Y = (R + X × 2^E) / 10^D
func unpackDRS0(sec7 []byte, p DRS0Params) ([]float64, error) {
	if len(sec7) < 5 {
		return nil, fmt.Errorf("drs0: section 7 too short")
	}
	data := sec7[5:] // skip 4-byte length + 1-byte section number

	n := p.N
	R := p.ReferenceValue
	scaleE := math.Ldexp(1.0, p.BinaryScaleFactor)
	scaleD := math.Pow(10, float64(p.DecimalScaleFactor))

	result := make([]float64, n)

	if p.Nbits == 0 {
		// Constant field: all values equal R / 10^D
		v := R / scaleD
		for i := range result {
			result[i] = v
		}
		return result, nil
	}

	br := newBitReader(data)
	for i := 0; i < n; i++ {
		x, err := br.read(p.Nbits)
		if err != nil {
			return nil, fmt.Errorf("drs0: reading value %d: %w", i, err)
		}
		result[i] = (R + scaleE*float64(x)) / scaleD
	}
	return result, nil
}
