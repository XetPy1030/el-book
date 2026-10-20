package weather

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"el-book/sdk/icons"
	"el-book/sdk/ru"
	"el-book/sdk/web"
)

// City — город прогноза.
type City struct {
	Name   string  `json:"name"`
	Region string  `json:"region,omitempty"` // область и страна: различить одноимённые
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
}

// forecast — ответ Open-Meteo.
type forecast struct {
	Current struct {
		Time     string  `json:"time"`
		Temp     float64 `json:"temperature_2m"`
		Feels    float64 `json:"apparent_temperature"`
		Humidity float64 `json:"relative_humidity_2m"`
		Code     int     `json:"weather_code"`
		Wind     float64 `json:"wind_speed_10m"`
		WindDir  float64 `json:"wind_direction_10m"`
		Pressure float64 `json:"surface_pressure"`
		IsDay    int     `json:"is_day"`
		Precip   float64 `json:"precipitation"`
	} `json:"current"`
	Hourly struct {
		Time   []string  `json:"time"`
		Temp   []float64 `json:"temperature_2m"`
		Code   []int     `json:"weather_code"`
		Chance []float64 `json:"precipitation_probability"`
	} `json:"hourly"`
	Daily struct {
		Time    []string  `json:"time"`
		Code    []int     `json:"weather_code"`
		Max     []float64 `json:"temperature_2m_max"`
		Min     []float64 `json:"temperature_2m_min"`
		Precip  []float64 `json:"precipitation_sum"`
		Chance  []float64 `json:"precipitation_probability_max"`
		Sunrise []string  `json:"sunrise"`
		Sunset  []string  `json:"sunset"`
	} `json:"daily"`
	Timezone  string `json:"timezone"`
	UTCOffset int    `json:"utc_offset_seconds"`
}

// loc — часовой пояс города: время в прогнозе — местное время города.
func (f *forecast) loc() *time.Location {
	return time.FixedZone(f.Timezone, f.UTCOffset)
}

// forecastURL — запрос прогноза: текущая погода, по часам на двое суток, по дням на неделю.
func forecastURL(c City) string {
	return web.URL("https://api.open-meteo.com/v1/forecast",
		"latitude", strconv.FormatFloat(c.Lat, 'f', 4, 64),
		"longitude", strconv.FormatFloat(c.Lon, 'f', 4, 64),
		"current", "temperature_2m,apparent_temperature,relative_humidity_2m,weather_code,wind_speed_10m,wind_direction_10m,surface_pressure,is_day,precipitation",
		"hourly", "temperature_2m,weather_code,precipitation_probability",
		"daily", "weather_code,temperature_2m_max,temperature_2m_min,precipitation_sum,precipitation_probability_max,sunrise,sunset",
		"wind_speed_unit", "ms",
		"timezone", "auto",
		"forecast_days", "7",
	)
}

// geoResult — ответ поиска городов.
type geoResult struct {
	Results []struct {
		Name    string  `json:"name"`
		Lat     float64 `json:"latitude"`
		Lon     float64 `json:"longitude"`
		Country string  `json:"country"`
		Admin1  string  `json:"admin1"`
	} `json:"results"`
}

func geoURL(name string) string {
	return web.URL("https://geocoding-api.open-meteo.com/v1/search", "name", name, "count", "8", "language", "ru", "format", "json")
}

// condition — описание и значок кода погоды WMO.
func condition(code int, day bool) (string, icons.Drawer) {
	switch {
	case code == 0:
		if !day {
			return "Ясно", icons.Moon
		}
		return "Ясно", icons.Sun
	case code == 1:
		return "Почти ясно", icons.CloudSun
	case code == 2:
		return "Переменная облачность", icons.CloudSun
	case code == 3:
		return "Пасмурно", icons.Cloud
	case code == 45 || code == 48:
		return "Туман", icons.Fog
	case code >= 51 && code <= 57:
		return "Морось", icons.Drop
	case code >= 61 && code <= 67:
		return "Дождь", icons.Rain
	case code >= 71 && code <= 77:
		return "Снег", icons.Snow
	case code >= 80 && code <= 82:
		return "Ливень", icons.Rain
	case code == 85 || code == 86:
		return "Снегопад", icons.Snow
	case code >= 95:
		return "Гроза", icons.Storm
	}
	return "Погода", icons.Cloud
}

// temp — «+8°», «−3°», «0°».
func temp(v float64) string {
	n := int(math.Round(v))
	switch {
	case n > 0:
		return fmt.Sprintf("+%d°", n)
	case n < 0:
		return fmt.Sprintf("−%d°", -n)
	}
	return "0°"
}

// windDir — откуда дует ветер: «С», «СЗ».
func windDir(deg float64) string {
	dirs := []string{"С", "СВ", "В", "ЮВ", "Ю", "ЮЗ", "З", "СЗ"}
	return dirs[int(math.Round(math.Mod(deg+360, 360)/45))%8]
}

// parseLocal разбирает время Open-Meteo «2026-10-19T14:00» в часовом поясе города.
func parseLocal(s string, loc *time.Location) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// hour — ближайшие часы прогноза.
type hour struct {
	at     time.Time
	temp   float64
	code   int
	chance float64
}

// nextHours — n часов прогноза начиная с текущего часа, через step часов.
func (f *forecast) nextHours(now time.Time, n, step int) []hour {
	loc := f.loc()
	now = now.In(loc)
	var out []hour
	for i, s := range f.Hourly.Time {
		t, ok := parseLocal(s, loc)
		if !ok || t.Before(now.Truncate(time.Hour)) || i >= len(f.Hourly.Temp) {
			continue
		}
		if len(out) > 0 && int(t.Sub(out[0].at).Hours())%step != 0 {
			continue
		}
		h := hour{at: t, temp: f.Hourly.Temp[i]}
		if i < len(f.Hourly.Code) {
			h.code = f.Hourly.Code[i]
		}
		if i < len(f.Hourly.Chance) {
			h.chance = f.Hourly.Chance[i]
		}
		out = append(out, h)
		if len(out) == n {
			break
		}
	}
	return out
}

// day — день прогноза.
type day struct {
	at              time.Time
	code            int
	lo, hi          float64
	precip, chance  float64
	sunrise, sunset string
}

func (f *forecast) days() []day {
	var out []day
	d := f.Daily
	for i, s := range d.Time {
		t, ok := parseLocal(s, f.loc())
		if !ok || i >= len(d.Max) || i >= len(d.Min) {
			continue
		}
		x := day{at: t, hi: d.Max[i], lo: d.Min[i]}
		if i < len(d.Code) {
			x.code = d.Code[i]
		}
		if i < len(d.Precip) {
			x.precip = d.Precip[i]
		}
		if i < len(d.Chance) {
			x.chance = d.Chance[i]
		}
		if i < len(d.Sunrise) && len(d.Sunrise[i]) >= 16 {
			x.sunrise = d.Sunrise[i][11:16]
		}
		if i < len(d.Sunset) && len(d.Sunset[i]) >= 16 {
			x.sunset = d.Sunset[i][11:16]
		}
		out = append(out, x)
	}
	return out
}

// dayName — «Сегодня», «Завтра», «пт, 24 октября».
func dayName(t, now time.Time) string {
	now = now.In(t.Location())
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	if y1 == y2 && m1 == m2 && d1 == d2 {
		return "Сегодня"
	}
	if tm := now.AddDate(0, 0, 1); tm.Year() == y1 && tm.Month() == m1 && tm.Day() == d1 {
		return "Завтра"
	}
	return ru.WeekdayShort(t.Weekday()) + ", " + ru.DayMonth(t, now)
}
