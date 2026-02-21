package grib2hrrr

import (
	"testing"
)

// ---------------------------------------------------------------------------
// readUintOctets
// ---------------------------------------------------------------------------

func TestReadUintOctets(t *testing.T) {
	cases := []struct {
		name string
		b    []byte
		want uint64
	}{
		{"1-byte zero", []byte{0x00}, 0},
		{"1-byte max", []byte{0xFF}, 255},
		{"1-byte mid", []byte{0x42}, 66},
		{"2-byte big-endian", []byte{0x01, 0x00}, 256},
		{"2-byte max", []byte{0xFF, 0xFF}, 65535},
		{"3-byte", []byte{0x01, 0x02, 0x03}, 0x010203},
		{"3-byte max", []byte{0xFF, 0xFF, 0xFF}, 0xFFFFFF},
		{"4-byte", []byte{0x00, 0x00, 0x01, 0x00}, 256},
		{"4-byte max", []byte{0xFF, 0xFF, 0xFF, 0xFF}, 0xFFFFFFFF},
		{"5-byte fallback", []byte{0x01, 0x02, 0x03, 0x04, 0x05}, 0x0102030405},
	}
	for _, tc := range cases {
		got := readUintOctets(tc.b)
		if got != tc.want {
			t.Errorf("readUintOctets(%v) [%s]: got %d, want %d", tc.b, tc.name, got, tc.want)
		}
	}
}

func TestReadUintOctetsEmptySliceReturnsZero(t *testing.T) {
	got := readUintOctets([]byte{})
	if got != 0 {
		t.Errorf("readUintOctets([]): got %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// readSignMagOctets
// ---------------------------------------------------------------------------

func TestReadSignMagOctets(t *testing.T) {
	cases := []struct {
		name string
		b    []byte
		want int64
	}{
		// 1-byte cases
		{"1-byte positive zero", []byte{0x00}, 0},
		{"1-byte negative zero", []byte{0x80}, 0}, // sign bit set, magnitude 0 = -0 = 0
		{"1-byte positive 1", []byte{0x01}, 1},
		{"1-byte positive 127", []byte{0x7F}, 127},
		{"1-byte negative 1", []byte{0x81}, -1},
		{"1-byte negative 127", []byte{0xFF}, -127},
		// 2-byte cases
		{"2-byte positive", []byte{0x00, 0x01}, 1},
		{"2-byte negative", []byte{0x80, 0x01}, -1},
		{"2-byte max positive", []byte{0x7F, 0xFF}, 32767},
		{"2-byte max negative", []byte{0xFF, 0xFF}, -32767},
		// 4-byte cases
		{"4-byte positive", []byte{0x00, 0x00, 0x00, 0x05}, 5},
		{"4-byte negative", []byte{0x80, 0x00, 0x00, 0x05}, -5},
		// empty slice
		{"empty", []byte{}, 0},
	}
	for _, tc := range cases {
		got := readSignMagOctets(tc.b)
		if got != tc.want {
			t.Errorf("readSignMagOctets(%v) [%s]: got %d, want %d", tc.b, tc.name, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// unpackDRS53 — error path tests (no network needed)
// ---------------------------------------------------------------------------

func TestUnpackDRS53TooShortReturnsError(t *testing.T) {
	// Section 7 must be at least 5 bytes (4-byte length + 1-byte section number).
	_, err := unpackDRS53([]byte{0x00, 0x00, 0x00}, DRS53Params{})
	if err == nil {
		t.Error("unpackDRS53 with 3-byte input: expected error, got nil")
	}
}

func TestUnpackDRS53UnsupportedOrderReturnsError(t *testing.T) {
	// Valid 5-byte header, order=3 (unsupported).
	sec7 := make([]byte, 5)
	p := DRS53Params{OrderSpatialDiff: 3, NOctetsExtra: 2}
	_, err := unpackDRS53(sec7, p)
	if err == nil {
		t.Error("unpackDRS53 with order=3: expected error, got nil")
	}
}

func TestUnpackDRS53OrderZeroReturnsError(t *testing.T) {
	sec7 := make([]byte, 5)
	p := DRS53Params{OrderSpatialDiff: 0, NOctetsExtra: 2}
	_, err := unpackDRS53(sec7, p)
	if err == nil {
		t.Error("unpackDRS53 with order=0: expected error, got nil")
	}
}

func TestUnpackDRS53BadNOctetsExtraReturnsError(t *testing.T) {
	// NOctetsExtra=0 is invalid (must be 1–4).
	sec7 := make([]byte, 5)
	p := DRS53Params{OrderSpatialDiff: 1, NOctetsExtra: 0}
	_, err := unpackDRS53(sec7, p)
	if err == nil {
		t.Error("unpackDRS53 with NOctetsExtra=0: expected error, got nil")
	}
}

func TestUnpackDRS53TooShortForExtraDescriptors(t *testing.T) {
	// order=2, m=4 → need (2+1)*4 = 12 extra bytes after the 5-byte header.
	// Provide only 5+2 = 7 bytes total (too short).
	sec7 := make([]byte, 7)
	p := DRS53Params{OrderSpatialDiff: 2, NOctetsExtra: 4}
	_, err := unpackDRS53(sec7, p)
	if err == nil {
		t.Error("unpackDRS53 with truncated extra descriptors: expected error, got nil")
	}
}

// TestUnpackDRS53RoundtripOrder1 exercises the full decode path with a hand-crafted
// 1-group, order-1 spatially differenced message with 3 values.
//
// Construction:
//
//	order=1, m=1, NG=1, nBits=4 (group ref bits), BitsGroupWidth=0, BitsGroupLength=0
//	initVals[0]=10, yMin=0
//	group ref=0, width=4, length=3 (LenLastGroup), values packed: 0,1,2
//	Decode: z = packed + yMin = [0,1,2]
//	undiff: undiff[0]=10, undiff[1]=10+0=10, undiff[2]=10+1=11, undiff[3]=10+2=12
//	Wait — order-1: undiff[0]=initVals[0], undiff[i]=z[i]+undiff[i-1]
//	z[0]=0, z[1]=1, z[2]=2 → undiff[0]=10, undiff[1]=0+10=10, undiff[2]=1+10=11 ? No:
//	After bias: z[i] = packed[i] + yMin. yMin=0, so z=[0,1,2]
//	undiff[0]=10, undiff[1]=z[1]+undiff[0]=1+10=11, undiff[2]=z[2]+undiff[1]=2+11=13
//	scale: R=0, E=0 (2^0=1), D=0 (10^0=1) → result = (0 + 1*undiff) / 1 = undiff
//	Expected: [10, 11, 13]
//
// Bit layout for section 7 payload (after 5-byte header):
//
//	Extra descriptors: initVals[0]=10 (1 byte, positive) = 0x0A
//	                   yMin=0 (1 byte, positive) = 0x00
//	Group refs (nBits=4): 1 group × 4 bits = 0x0_ (ref=0 → 0000)
//	Then align to byte.
//	Group widths (BitsGroupWidth=4): 1 group × 4 bits = width delta=0 (RefGroupWidth=4, delta=0 → 0000)
//	Then align to byte.
//	Group lengths (BitsGroupLength=4): 1 group × 4 bits = length encoding=0 (consumed, LenLastGroup overrides)
//	Then align to byte.
//	Data values: width=4, length=3: 3 × 4 bits = 12 bits
//	  val[0]=0: 0000, val[1]=1: 0001, val[2]=2: 0010 → 0000 0001 0010 → pad to 2 bytes: 0x01 0x20
//
// Full payload bytes after the 5-byte section header:
//
//	[0x0A] [0x00]          ← initVals[0]=10, yMin=0
//	[0x00]                 ← gref=0 in 4 bits + 4-bit pad → 0x00 (after align)
//	[0x00]                 ← width delta=0 in 4 bits + 4-bit pad → 0x00 (after align)
//	[0x00]                 ← length encoding in 4 bits + 4-bit pad → 0x00 (after align)
//	[0x01] [0x20]          ← 3 values × 4 bits: 0000 0001 0010 0000 (last nibble pad)
func TestUnpackDRS53RoundtripOrder1(t *testing.T) {
	payload := []byte{
		0x0A, 0x00, // initVals[0]=10, yMin=0
		0x00,       // gref=0 in 4 bits, aligned to 1 byte
		0x00,       // width delta=0 (RefGroupWidth=4, total=4), aligned
		0x00,       // length field consumed but ignored (LenLastGroup=3 overrides), aligned
		0x01, 0x20, // values 0,1,2 packed as 4-bit nibbles: 0000 0001 0010 0000
	}
	// 5-byte section header + payload
	sec7 := append([]byte{0x00, 0x00, 0x00, 0x00, 0x07}, payload...)

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

	result, err := unpackDRS53(sec7, p)
	if err != nil {
		t.Fatalf("unpackDRS53 roundtrip order-1: %v", err)
	}

	want := []float64{10, 11, 13}
	if len(result) != len(want) {
		t.Fatalf("result length: got %d, want %d", len(result), len(want))
	}
	for i, w := range want {
		if result[i] != w {
			t.Errorf("result[%d]: got %g, want %g", i, result[i], w)
		}
	}
}
