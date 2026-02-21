// Package grib2hrrr decodes NOAA HRRR GRIB2 files using GDT 3.30 (Lambert
// conformal) grids and DRS Template 5.3 (complex packing + spatial differencing).
package grib2hrrr

import "math"

const earthRadiusM = 6371229.0 // shape-of-earth=6 (sphere), HRRR standard

// LambertGrid holds parsed GDT 3.30 parameters.
type LambertGrid struct {
	Ni, Nj         int
	La1, Lo1       float64 // first grid point, signed degrees (La1 SW corner)
	LoV            float64 // central meridian, signed degrees
	Latin1, Latin2 float64 // standard parallels, degrees
	Dx, Dy         float64 // grid spacing, metres
	ScanMode       byte
}

func (g *LambertGrid) n() float64 {
	if g.Latin1 == g.Latin2 {
		return math.Sin(toRad(g.Latin1))
	}
	φ1 := toRad(g.Latin1)
	φ2 := toRad(g.Latin2)
	return math.Log(math.Cos(φ1)/math.Cos(φ2)) /
		math.Log(math.Tan(math.Pi/4+φ2/2)/math.Tan(math.Pi/4+φ1/2))
}

func (g *LambertGrid) bigF() float64 {
	n := g.n()
	φ1 := toRad(g.Latin1)
	return math.Cos(φ1) * math.Pow(math.Tan(math.Pi/4+φ1/2), n) / n
}

// rho returns the Lambert cone distance (metres) from the pole for a given latitude.
func (g *LambertGrid) rho(latDeg float64) float64 {
	n := g.n()
	F := g.bigF()
	φ := toRad(latDeg)
	return earthRadiusM * F / math.Pow(math.Tan(math.Pi/4+φ/2), n)
}

// refXY returns the Lambert Cartesian coordinates of the grid origin (La1,Lo1).
// Convention: x east-positive, y = -ρ*cos(θ) so y is north-positive.
func (g *LambertGrid) refXY() (x0, y0 float64) {
	n := g.n()
	ρ0 := g.rho(g.La1)
	θ0 := n * toRad(NormLon(g.Lo1)-NormLon(g.LoV))
	x0 = ρ0 * math.Sin(θ0)
	y0 = -ρ0 * math.Cos(θ0) // y increases northward
	return
}

// LatLonToIJ maps (lat°N, lon°E signed) → nearest grid indices (i,j).
// i increases eastward, j increases northward (scanning mode 0x40).
func (g *LambertGrid) LatLonToIJ(lat, lon float64) (i, j int) {
	n := g.n()
	ρ := g.rho(lat)
	θ := n * toRad(NormLon(lon)-NormLon(g.LoV))
	x := ρ * math.Sin(θ)
	y := -ρ * math.Cos(θ)

	x0, y0 := g.refXY()
	fi := (x - x0) / g.Dx
	fj := (y - y0) / g.Dy
	i = int(math.Round(fi))
	j = int(math.Round(fj))
	return
}

// IjToLatLon maps grid indices (i,j) → (lat°N, lon°E signed).
func (g *LambertGrid) IjToLatLon(i, j int) (lat, lon float64) {
	n := g.n()
	F := g.bigF()
	x0, y0 := g.refXY()

	x := x0 + float64(i)*g.Dx
	y := y0 + float64(j)*g.Dy

	ρ := math.Sqrt(x*x + y*y)
	if ρ == 0 {
		return 90, NormLon(g.LoV)
	}
	// x = ρ*sin(θ), -y = ρ*cos(θ) → θ = atan2(x, -y)
	θ := math.Atan2(x, -y)
	φ := 2*math.Atan(math.Pow(earthRadiusM*F/ρ, 1/n)) - math.Pi/2
	lat = toDeg(φ)
	lon = NormLon(g.LoV) + toDeg(θ)/n
	return
}

// Lookup returns the float64 value at (lat, lon) by nearest-neighbour from vals.
// vals is a flat row-major slice: index = j*Ni + i.
// Returns math.NaN() if the point falls outside the grid.
func (g *LambertGrid) Lookup(lat, lon float64, vals []float64) float64 {
	i, j := g.LatLonToIJ(lat, lon)
	if i < 0 || i >= g.Ni || j < 0 || j >= g.Nj {
		return math.NaN()
	}
	return vals[j*g.Ni+i]
}

// helpers
func toRad(d float64) float64 { return d * math.Pi / 180 }
func toDeg(r float64) float64 { return r * 180 / math.Pi }

// NormLon converts a 0-360 longitude to -180..+180.
// Exported so callers can normalize GRIB2 longitudes (which use 0-360 convention).
// Issue #11: previously unexported, duplicated in test file.
func NormLon(lon float64) float64 {
	if lon > 180 {
		return lon - 360
	}
	return lon
}
