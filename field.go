package grib2hrrr

// Field is a decoded GRIB2 field: a Lambert conformal grid + float64 values.
// Values are stored row-major: vals[j*Grid.Ni + i].
type Field struct {
	Grid LambertGrid
	Vals []float64
}

// Lookup returns the nearest-neighbour value at (lat°N, lon°E).
func (f *Field) Lookup(lat, lon float64) float64 {
	return f.Grid.Lookup(lat, lon, f.Vals)
}
