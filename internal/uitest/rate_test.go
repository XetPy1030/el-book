package uitest

import (
	"strings"
	"testing"
	"time"

	"el-book/sdk/device"
)

// TestStrokeUpdateRate: живой след в рисовалке уходит на панель не чаще раза в 40 мс. На книге
// обновление на каждое движение пальца перегружало контроллер e-ink («no luts available» в
// журнале ядра), и след отставал от пальца.
func TestStrokeUpdateRate(t *testing.T) {
	if testing.Short() {
		t.Skip("UI-тесты идут несколько секунд; пропущены в -short")
	}
	t.Parallel()
	dev, _ := start(t)
	run := func(script string) {
		t.Helper()
		if err := dev.RunScript(strings.NewReader(script), nil); err != nil {
			t.Fatal(err)
		}
	}
	run("tap 263 676\ntap 1270 70\ntap 720 810\nwait 2700\n") // новая тетрадь в линейку
	from := 0
	for _, r := range dev.History() {
		from = max(from, r.Seq)
	}
	began := time.Now()
	run("drag 150 400 1250 400 1250 700 150 700 150 1000 1250 1000\n") // около 300 движений
	took := time.Since(began)

	strokes := 0
	for _, r := range dev.History() {
		if r.Seq > from && r.Waveform == device.WaveformA2.String() {
			strokes++
		}
	}
	limit := int(took/(40*time.Millisecond)) + 5
	if strokes < 5 || strokes > limit {
		t.Fatalf("за %v ведения пальцем — %d обновлений следа, ожидалось от 5 до %d", took.Round(time.Millisecond), strokes, limit)
	}
}
