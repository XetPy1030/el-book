package weather

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// response — ответ Open-Meteo в том виде, как его присылает сервер: поля, которые приложение
// не читает, тоже на месте.
const response = `{"latitude":55.75,"longitude":37.62,"utc_offset_seconds":10800,"timezone":"Europe/Moscow",
"current":{"time":"2026-10-19T14:00","interval":900,"temperature_2m":8.2,"apparent_temperature":5.9,
 "relative_humidity_2m":64,"weather_code":2,"wind_speed_10m":3.4,"wind_direction_10m":225,
 "surface_pressure":1012.6,"is_day":1,"precipitation":0.0},
"hourly":{"time":["2026-10-19T12:00","2026-10-19T13:00","2026-10-19T14:00","2026-10-19T15:00",
 "2026-10-19T16:00","2026-10-19T17:00","2026-10-19T18:00"],
 "temperature_2m":[7.1,7.8,8.4,8.5,8.2,7.5,6.6],"weather_code":[2,2,2,3,3,61,61],
 "precipitation_probability":[10,10,10,20,30,60,80]},
"daily":{"time":["2026-10-19","2026-10-20","2026-10-21"],"weather_code":[2,61,71],
 "temperature_2m_max":[8.5,7.0,1.0],"temperature_2m_min":[1.5,3.0,-3.0],
 "precipitation_sum":[0,4.2,2.1],"precipitation_probability_max":[10,80,70],
 "sunrise":["2026-10-19T07:08","2026-10-20T07:10","2026-10-21T07:12"],
 "sunset":["2026-10-19T17:19","2026-10-20T17:17","2026-10-21T17:15"]}}`

func parse(t *testing.T) *forecast {
	t.Helper()
	var f forecast
	if err := json.Unmarshal([]byte(response), &f); err != nil {
		t.Fatal(err)
	}
	return &f
}

func TestForecastHoursInCityZone(t *testing.T) {
	f := parse(t)
	// Книга живёт в другом поясе: прогноз всё равно считается по часам города.
	now := time.Date(2026, 10, 19, 15, 30, 0, 0, time.FixedZone("Самара", 4*3600)) // 14:30 в Москве
	hours := f.nextHours(now, 3, 2)
	var got []string
	for _, h := range hours {
		got = append(got, h.at.Format("15:04")+" "+temp(h.temp))
	}
	if want := "14:00 +8°|16:00 +8°|18:00 +7°"; strings.Join(got, "|") != want {
		t.Fatalf("часы прогноза %q, нужно %q", strings.Join(got, "|"), want)
	}
}

func TestForecastDays(t *testing.T) {
	f := parse(t)
	days := f.days()
	if len(days) != 3 {
		t.Fatalf("дней %d", len(days))
	}
	now := time.Date(2026, 10, 19, 14, 5, 0, 0, f.loc())
	if dayName(days[0].at, now) != "Сегодня" || dayName(days[1].at, now) != "Завтра" {
		t.Errorf("названия дней: %q, %q", dayName(days[0].at, now), dayName(days[1].at, now))
	}
	if !strings.HasPrefix(dayName(days[2].at, now), "ср, 21") {
		t.Errorf("третий день: %q", dayName(days[2].at, now))
	}
	if days[0].sunrise != "07:08" || days[0].sunset != "17:19" {
		t.Errorf("восход и закат: %s, %s", days[0].sunrise, days[0].sunset)
	}
	if text, _ := condition(days[2].code, true); text != "Снег" || temp(days[2].lo) != "−3°" {
		t.Errorf("третий день: %s, минимум %s", text, temp(days[2].lo))
	}
}

func TestFormatting(t *testing.T) {
	cases := map[float64]string{8.4: "+8°", -0.4: "0°", -2.6: "−3°", 0: "0°"}
	for v, want := range cases {
		if got := temp(v); got != want {
			t.Errorf("temp(%v) = %q, нужно %q", v, got, want)
		}
	}
	dirs := map[float64]string{0: "С", 225: "ЮЗ", 350: "С", 90: "В", -45: "СЗ"}
	for deg, want := range dirs {
		if got := windDir(deg); got != want {
			t.Errorf("windDir(%v) = %q, нужно %q", deg, got, want)
		}
	}
	if text, _ := condition(0, false); text != "Ясно" {
		t.Errorf("ясная ночь: %q", text)
	}
	if text, _ := condition(95, true); text != "Гроза" {
		t.Errorf("гроза: %q", text)
	}
}
