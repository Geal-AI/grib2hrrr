package grib2hrrr

import (
	"encoding/binary"
	"math"
	"testing"
)

// buildDRS0Section builds a minimal Section 5 (DRS Template 5.0) byte slice.
func buildDRS0Section(n int, R float32, E, D int16, nBits int) []byte {
	// Total section length: 5 (header) + 4 (N) + 2 (tmpl) + 10 (template data) = 21 bytes
	sec := make([]byte, 21)
	binary.BigEndian.PutUint32(sec[0:4], 21)
	sec[4] = 5
	binary.BigEndian.PutUint32(sec[5:9], uint32(n))
	binary.BigEndian.PutUint16(sec[9:11], 0) // template 5.0
	binary.BigEndian.PutUint32(sec[11:15], math.Float32bits(R))
	// Encode E and D as sign-magnitude 2-byte values
	encSM := func(v int16) uint16 {
		if v < 0 {
			return 0x8000 | uint16(-v)
		}
		return uint16(v)
	}
	binary.BigEndian.PutUint16(sec[15:17], encSM(E))
	binary.BigEndian.PutUint16(sec[17:19], encSM(D))
	sec[19] = byte(nBits)
	sec[20] = 0 // type: floating point
	return sec
}

// buildDRS0Sec7 packs n values of nBits each into a Section 7 byte slice.
func buildDRS0Sec7(vals []uint64, nBits int) []byte {
	// Calculate number of bytes needed for packed data
	nBitsTotal := len(vals) * nBits
	nBytes := (nBitsTotal + 7) / 8
	data := make([]byte, nBytes)

	pos := 0
	for _, v := range vals {
		for b := nBits - 1; b >= 0; b-- {
			bit := (v >> uint(b)) & 1
			byteIdx := pos / 8
			bitIdx := 7 - (pos % 8)
			data[byteIdx] |= byte(bit) << uint(bitIdx)
			pos++
		}
	}

	sec := make([]byte, 5+len(data))
	binary.BigEndian.PutUint32(sec[0:4], uint32(len(sec)))
	sec[4] = 7
	copy(sec[5:], data)
	return sec
}

func TestParseDRS0TooShort(t *testing.T) {
	sec := make([]byte, 15) // need 21
	_, err := parseDRS0(sec)
	if err == nil {
		t.Fatal("expected error for too-short section 5")
	}
}

func TestParseDRS0NbitsTooLarge(t *testing.T) {
	sec := buildDRS0Section(100, 0, 0, 0, maxBitWidth+1)
	_, err := parseDRS0(sec)
	if err == nil {
		t.Fatalf("expected error for Nbits=%d > %d", maxBitWidth+1, maxBitWidth)
	}
}

func TestParseDRS0NTooLarge(t *testing.T) {
	sec := buildDRS0Section(0, 0, 0, 0, 16)
	// Overwrite N with maxTotal+1
	binary.BigEndian.PutUint32(sec[5:9], uint32(maxTotal+1))
	_, err := parseDRS0(sec)
	if err == nil {
		t.Fatalf("expected error for N > maxTotal")
	}
}

func TestParseDRS0Valid(t *testing.T) {
	sec := buildDRS0Section(1000, -5.5, -1, 0, 16)
	p, err := parseDRS0(sec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.N != 1000 {
		t.Errorf("N: got %d, want 1000", p.N)
	}
	if p.Nbits != 16 {
		t.Errorf("Nbits: got %d, want 16", p.Nbits)
	}
	if p.BinaryScaleFactor != -1 {
		t.Errorf("E: got %d, want -1", p.BinaryScaleFactor)
	}
}

func TestUnpackDRS0TooShort(t *testing.T) {
	_, err := unpackDRS0([]byte{0, 0, 0, 1}, DRS0Params{N: 1, Nbits: 8})
	if err == nil {
		t.Fatal("expected error for sec7 too short")
	}
}

func TestUnpackDRS0ConstantField(t *testing.T) {
	// Nbits == 0: all values equal R / 10^D
	sec7 := make([]byte, 5) // empty data section
	binary.BigEndian.PutUint32(sec7[0:4], 5)
	sec7[4] = 7

	p := DRS0Params{N: 5, Nbits: 0, ReferenceValue: 273.15, DecimalScaleFactor: 0}
	vals, err := unpackDRS0(sec7, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vals) != 5 {
		t.Fatalf("len: got %d, want 5", len(vals))
	}
	for i, v := range vals {
		if math.Abs(v-273.15) > 1e-4 {
			t.Errorf("vals[%d] = %g, want ~273.15", i, v)
		}
	}
}

func TestUnpackDRS0Roundtrip(t *testing.T) {
	// Pack values [0, 1, 2, ..., 9] with R=0, E=0, D=0, nBits=8
	// Y = (0 + X * 2^0) / 10^0 = X
	packed := []uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	sec7 := buildDRS0Sec7(packed, 8)

	p := DRS0Params{N: 10, Nbits: 8, ReferenceValue: 0, BinaryScaleFactor: 0, DecimalScaleFactor: 0}
	vals, err := unpackDRS0(sec7, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, v := range vals {
		if math.Abs(v-float64(i)) > 1e-6 {
			t.Errorf("vals[%d] = %g, want %d", i, v, i)
		}
	}
}

func TestUnpackDRS0ScaleFactors(t *testing.T) {
	// R=100, E=-1 (divide by 2), D=1 (divide by 10), nBits=8
	// Y = (100 + X * 2^-1) / 10^1 = (100 + X/2) / 10
	// For X=0: Y = 10.0
	// For X=20: Y = (100 + 10) / 10 = 11.0
	packed := []uint64{0, 20}
	sec7 := buildDRS0Sec7(packed, 8)

	p := DRS0Params{
		N: 2, Nbits: 8,
		ReferenceValue: 100, BinaryScaleFactor: -1, DecimalScaleFactor: 1,
	}
	vals, err := unpackDRS0(sec7, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if math.Abs(vals[0]-10.0) > 1e-5 {
		t.Errorf("vals[0] = %g, want 10.0", vals[0])
	}
	if math.Abs(vals[1]-11.0) > 1e-5 {
		t.Errorf("vals[1] = %g, want 11.0", vals[1])
	}
}

func TestDecodeMessageDRS0(t *testing.T) {
	// Build a minimal valid GRIB2 message with DRS Template 5.0.
	// Grid: 2×2 Lambert (Ni=2, Nj=2), values [1, 2, 3, 4] with 8-bit simple packing.
	n := 4

	// Section 0: indicator (16 bytes)
	sec0 := []byte{
		'G', 'R', 'I', 'B', 0, 0, // reserved
		0,                      // discipline: meteorological
		2,                      // edition: 2
		0, 0, 0, 0, 0, 0, 0, 0, // total length (we'll fix below)
	}

	// Section 1: identification (21 bytes minimum)
	sec1 := make([]byte, 21)
	binary.BigEndian.PutUint32(sec1[0:4], 21)
	sec1[4] = 1

	// Section 3: grid definition — Lambert 2×2 (14 + 67 = 81 bytes)
	sec3 := make([]byte, 81)
	binary.BigEndian.PutUint32(sec3[0:4], 81)
	sec3[4] = 3
	sec3[5] = 0 // source of grid definition
	binary.BigEndian.PutUint32(sec3[6:10], uint32(n))
	// GDT 3.30 at sec3[12:14]
	binary.BigEndian.PutUint16(sec3[12:14], 30)
	g := sec3[14:]
	g[0] = 6 // shape of earth = sphere
	// g[1..15]: radius/major/minor = 0
	binary.BigEndian.PutUint32(g[16:20], 2)                       // Ni
	binary.BigEndian.PutUint32(g[20:24], 2)                       // Nj
	binary.BigEndian.PutUint32(g[24:28], uint32(int32(35000000))) // La1 = 35°N
	binary.BigEndian.PutUint32(g[28:32], uint32(262000000))       // Lo1 = 262° (= -98°W)
	g[32] = 0                                                     // resolution flags
	binary.BigEndian.PutUint32(g[33:37], uint32(int32(38000000))) // LaD = 38°N
	binary.BigEndian.PutUint32(g[37:41], uint32(262000000))       // LoV = 262°
	binary.BigEndian.PutUint32(g[41:45], 3000000)                 // Dx = 3000 m
	binary.BigEndian.PutUint32(g[45:49], 3000000)                 // Dy = 3000 m
	g[49] = 0                                                     // projection centre
	g[50] = 0x40                                                  // scan mode
	binary.BigEndian.PutUint32(g[51:55], uint32(int32(38500000))) // Latin1
	binary.BigEndian.PutUint32(g[55:59], uint32(int32(38500000))) // Latin2
	// g[59..66]: southern pole lat/lon = 0

	// Section 4: product definition (9 bytes minimum)
	sec4 := make([]byte, 9)
	binary.BigEndian.PutUint32(sec4[0:4], 9)
	sec4[4] = 4

	// Section 5: DRS Template 5.0, N=4, R=0, E=0, D=0, nBits=8
	sec5 := buildDRS0Section(n, 0, 0, 0, 8)

	// Section 6: bitmap section (6 bytes, bitmap indicator = 255 = no bitmap)
	sec6 := make([]byte, 6)
	binary.BigEndian.PutUint32(sec6[0:4], 6)
	sec6[4] = 6
	sec6[5] = 255

	// Section 7: pack values [1, 2, 3, 4] as 8-bit unsigned integers
	sec7 := buildDRS0Sec7([]uint64{1, 2, 3, 4}, 8)

	// End marker
	end := []byte{'7', '7', '7', '7'}

	// Assemble message
	msg := concat(sec0, sec1, sec3, sec4, sec5, sec6, sec7, end)

	// Fix Section 0 total length
	binary.BigEndian.PutUint64(msg[8:16], uint64(len(msg)))

	field, err := DecodeMessage(msg)
	if err != nil {
		t.Fatalf("DecodeMessage: %v", err)
	}
	if len(field.Vals) != n {
		t.Fatalf("len(Vals) = %d, want %d", len(field.Vals), n)
	}
	want := []float64{1, 2, 3, 4}
	for i, v := range field.Vals {
		if math.Abs(v-want[i]) > 1e-5 {
			t.Errorf("Vals[%d] = %g, want %g", i, v, want[i])
		}
	}
}

// concat appends all slices together.
func concat(slices ...[]byte) []byte {
	var out []byte
	for _, s := range slices {
		out = append(out, s...)
	}
	return out
}
