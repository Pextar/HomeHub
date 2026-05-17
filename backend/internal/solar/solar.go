// Package solar computes sunrise and sunset times for a given location
// and date using the NOAA Solar Position Algorithm.
//
// The math is accurate to roughly one minute at temperate latitudes,
// which is plenty for "turn on the lamp at sunset" automation. Near the
// polar circles around the solstices the sun may never rise or set; in
// that case Times returns ok=false.
package solar

import (
	"math"
	"time"
)

// Times returns the sunrise and sunset on the local date of t at the
// given location. Returned values share t's time zone.
//
// ok is false when the sun does not cross the horizon on that date at
// that latitude (polar day/night).
func Times(t time.Time, latDeg, lonDeg float64) (sunrise, sunset time.Time, ok bool) {
	year, month, day := t.Date()
	loc := t.Location()
	// Reference instant: local noon, so the fractional-year terms land
	// in the middle of the day regardless of time zone.
	noonLocal := time.Date(year, month, day, 12, 0, 0, 0, loc)
	noonUTC := noonLocal.UTC()
	dayOfYear := noonUTC.YearDay()

	gamma := 2 * math.Pi / 365 * float64(dayOfYear-1)
	eqtime := 229.18 * (0.000075 +
		0.001868*math.Cos(gamma) -
		0.032077*math.Sin(gamma) -
		0.014615*math.Cos(2*gamma) -
		0.040849*math.Sin(2*gamma))
	decl := 0.006918 -
		0.399912*math.Cos(gamma) +
		0.070257*math.Sin(gamma) -
		0.006758*math.Cos(2*gamma) +
		0.000907*math.Sin(2*gamma) -
		0.002697*math.Cos(3*gamma) +
		0.00148*math.Sin(3*gamma)

	latRad := latDeg * math.Pi / 180
	// 90.833° accounts for the solar disk radius and average refraction.
	zenith := 90.833 * math.Pi / 180
	cosH := (math.Cos(zenith) - math.Sin(latRad)*math.Sin(decl)) /
		(math.Cos(latRad) * math.Cos(decl))
	if cosH > 1 || cosH < -1 {
		return time.Time{}, time.Time{}, false
	}
	hourAngle := math.Acos(cosH) * 180 / math.Pi // degrees

	// Minutes since UTC midnight of the reference date.
	sunriseMin := 720 - 4*(lonDeg+hourAngle) - eqtime
	sunsetMin := 720 - 4*(lonDeg-hourAngle) - eqtime

	utcMidnight := time.Date(noonUTC.Year(), noonUTC.Month(), noonUTC.Day(), 0, 0, 0, 0, time.UTC)
	sunriseUTC := utcMidnight.Add(time.Duration(sunriseMin * float64(time.Minute)))
	sunsetUTC := utcMidnight.Add(time.Duration(sunsetMin * float64(time.Minute)))

	return sunriseUTC.In(loc), sunsetUTC.In(loc), true
}
