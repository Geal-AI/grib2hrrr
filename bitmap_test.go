package grib2hrrr

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestBitmapBit(t *testing.T) {
	// 0b10110000 = 0xB0: bits 7,5,4 set → points 0,2,3 have data
	bitmap := []byte{0xB0}
	cases := []struct {
		i    int
		want bool
	}{
		{0, true},
		{1, false},
		{2, true},
		{3, true},
		{4, false},
		{5, false},
		{6, false},
		{7, false},
	}
	for _, c := range cases {
		got := bitmapBit(bitmap, c.i)
		if got != c.want {
			t.Errorf("bitmapBit(%d) = %v, want %v", c.i, got, c.want)
		}
	}
}

func TestApplyBitmapAllSet(t *testing.T) {
	vals := []float64{1, 2, 3, 4, 5, 6, 7, 8}
	bitmap := []byte{0xFF} // all 8 bits set
	result, err := applyBitmap(vals, bitmap, 8)
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range result {
		if v != vals[i] {
			t.Errorf("result[%d] = %g, want %g", i, v, vals[i])
		}
	}
}

func TestApplyBitmapSomeMissing(t *testing.T) {
	// 0x90 = 0b10010000: bits 7 and 4 set → points 0 and 3 have data
	bitmap := []byte{0x90}
	vals := []float64{10, 40}
	result, err := applyBitmap(vals, bitmap, 8)
	if err != nil {
		t.Fatal(err)
	}
	wantValid := map[int]float64{0: 10, 3: 40}
	for i, v := range result {
		if wantV, ok := wantValid[i]; ok {
			if v != wantV {
				t.Errorf("result[%d] = %g, want %g", i, v, wantV)
			}
		} else if !math.IsNaN(v) {
			t.Errorf("result[%d] = %g, want NaN", i, v)
		}
	}
}

func TestApplyBitmapAllMissing(t *testing.T) {
	bitmap := []byte{0x00}
	result, err := applyBitmap(nil, bitmap, 8)
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range result {
		if !math.IsNaN(v) {
			t.Errorf("result[%d] = %g, want NaN", i, v)
		}
	}
}

func TestApplyBitmapMismatch(t *testing.T) {
	// 3 set bits but only 2 values → error
	bitmap := []byte{0b11100000}
	_, err := applyBitmap([]float64{1, 2}, bitmap, 8)
	if err == nil {
		t.Fatal("expected error for mismatched count")
	}
}

func TestApplyBitmapMultiByteGrid(t *testing.T) {
	// 16-point grid, bitmap = 0xFF00 → points 0-7 have data, 8-15 are NaN
	bitmap := []byte{0xFF, 0x00}
	vals := make([]float64, 8)
	for i := range vals {
		vals[i] = float64(i + 1)
	}
	result, err := applyBitmap(vals, bitmap, 16)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if result[i] != float64(i+1) {
			t.Errorf("result[%d] = %g, want %g", i, result[i], float64(i+1))
		}
	}
	for i := 8; i < 16; i++ {
		if !math.IsNaN(result[i]) {
			t.Errorf("result[%d] = %g, want NaN", i, result[i])
		}
	}
}

// TestDecodeMessageWithBitmap builds a minimal GRIB2 message with a bitmap
// (Section 6 flag=0) and verifies that missing grid points become NaN.
func TestDecodeMessageWithBitmap(t *testing.T) {
	// Grid: 2×2 (4 points). Bitmap 0x90 = 0b10010000 → points 0 and 3 have data.
	// DRS 5.0: N=2, pack [1, 4] as 8-bit. Points 1,2 → NaN.

	// Section 0
	sec0 := []byte{
		'G', 'R', 'I', 'B', 0, 0,
		0,                      // discipline: meteorological
		2,                      // edition 2
		0, 0, 0, 0, 0, 0, 0, 0, // total length (fixed below)
	}

	// Section 1 (21 bytes)
	sec1 := make([]byte, 21)
	binary.BigEndian.PutUint32(sec1[0:4], 21)
	sec1[4] = 1

	// Section 3: Lambert 2×2 (81 bytes, same layout as TestDecodeMessageDRS0)
	sec3 := make([]byte, 81)
	binary.BigEndian.PutUint32(sec3[0:4], 81)
	sec3[4] = 3
	binary.BigEndian.PutUint32(sec3[6:10], 4)   // 4 grid points
	binary.BigEndian.PutUint16(sec3[12:14], 30) // GDT 3.30
	g := sec3[14:]
	g[0] = 6
	binary.BigEndian.PutUint32(g[16:20], 2)                       // Ni=2
	binary.BigEndian.PutUint32(g[20:24], 2)                       // Nj=2
	binary.BigEndian.PutUint32(g[24:28], uint32(int32(35000000))) // La1
	binary.BigEndian.PutUint32(g[28:32], uint32(262000000))       // Lo1
	binary.BigEndian.PutUint32(g[33:37], uint32(int32(38000000))) // LaD
	binary.BigEndian.PutUint32(g[37:41], uint32(262000000))       // LoV
	binary.BigEndian.PutUint32(g[41:45], 3000000)                 // Dx
	binary.BigEndian.PutUint32(g[45:49], 3000000)                 // Dy
	g[50] = 0x40
	binary.BigEndian.PutUint32(g[51:55], uint32(int32(38500000))) // Latin1
	binary.BigEndian.PutUint32(g[55:59], uint32(int32(38500000))) // Latin2

	// Section 4 (9 bytes)
	sec4 := make([]byte, 9)
	binary.BigEndian.PutUint32(sec4[0:4], 9)
	sec4[4] = 4

	// Section 5: DRS 5.0, N=2 (only 2 packed values due to bitmap)
	sec5 := buildDRS0Section(2, 0, 0, 0, 8)

	// Section 6: bitmap present (flag=0), bitmap byte = 0x90 → points 0,3 valid
	sec6 := make([]byte, 7)
	binary.BigEndian.PutUint32(sec6[0:4], 7)
	sec6[4] = 6
	sec6[5] = 0    // bitmap indicator
	sec6[6] = 0x90 // 0b10010000: bits 7 and 4 set → grid points 0 and 3

	// Section 7: pack [1, 4] as 8-bit unsigned
	sec7 := buildDRS0Sec7([]uint64{1, 4}, 8)

	end := []byte{'7', '7', '7', '7'}
	msg := concat(sec0, sec1, sec3, sec4, sec5, sec6, sec7, end)
	binary.BigEndian.PutUint64(msg[8:16], uint64(len(msg)))

	field, err := DecodeMessage(msg)
	if err != nil {
		t.Fatalf("DecodeMessage: %v", err)
	}
	if len(field.Vals) != 4 {
		t.Fatalf("len(Vals) = %d, want 4", len(field.Vals))
	}

	// Point 0: value 1
	if math.Abs(field.Vals[0]-1.0) > 1e-5 {
		t.Errorf("Vals[0] = %g, want 1.0", field.Vals[0])
	}
	// Point 1: NaN (bitmap=0)
	if !math.IsNaN(field.Vals[1]) {
		t.Errorf("Vals[1] = %g, want NaN", field.Vals[1])
	}
	// Point 2: NaN (bitmap=0)
	if !math.IsNaN(field.Vals[2]) {
		t.Errorf("Vals[2] = %g, want NaN", field.Vals[2])
	}
	// Point 3: value 4
	if math.Abs(field.Vals[3]-4.0) > 1e-5 {
		t.Errorf("Vals[3] = %g, want 4.0", field.Vals[3])
	}
}
