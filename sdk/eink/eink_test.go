package eink

import (
	"image"
	"testing"
	"time"

	"el-book/sdk/device"
)

var screen = image.Rect(0, 0, 1440, 1920)

func TestCleanWinsEverything(t *testing.T) {
	p := NewPolicy()
	got := p.Plan(screen, []Damage{
		{image.Rect(0, 0, 10, 10), Feedback},
		{image.Rect(5, 5, 50, 50), Clean},
	})
	if len(got) != 1 || !got[0].Full || got[0].Waveform != device.WaveformGC16 || got[0].Rect != screen {
		t.Fatalf("ожидалось одно полное GC16 на экран, получено %+v", got)
	}
}

func TestClassesMapToWaveforms(t *testing.T) {
	p := NewPolicy()
	got := p.Plan(screen, []Damage{
		{image.Rect(0, 0, 100, 100), UI},
		{image.Rect(500, 500, 600, 600), Feedback},
		{image.Rect(900, 900, 920, 920), Stroke},
		{image.Rect(0, 1500, 300, 1800), Content},
	})
	want := []device.Waveform{device.WaveformDU, device.WaveformA2, device.WaveformGL16, device.WaveformGC16}
	if len(got) != len(want) {
		t.Fatalf("ожидалось %d обновлений, получено %+v", len(want), got)
	}
	for i, w := range want {
		if got[i].Waveform != w || got[i].Full {
			t.Errorf("обновление %d: %v full=%v, ожидалось %v частичное", i, got[i].Waveform, got[i].Full, w)
		}
	}
}

func TestFastUpdatesInsideRedrawAreDropped(t *testing.T) {
	p := NewPolicy()
	got := p.Plan(screen, []Damage{
		{image.Rect(10, 10, 50, 50), Feedback},
		{image.Rect(60, 60, 90, 90), Stroke},
		{image.Rect(0, 0, 200, 200), UI},
		{image.Rect(500, 500, 600, 600), Stroke},
	})
	if len(got) != 2 || got[0].Waveform != device.WaveformA2 || got[1].Waveform != device.WaveformGL16 {
		t.Fatalf("отклик и штрих внутри перерисовки должны пропасть, штрих снаружи — остаться: %+v", got)
	}
}

func TestFlashAfterSeriesOfMajorUpdates(t *testing.T) {
	p := NewPolicy()
	p.FlashEvery = 3
	for i := 1; i <= 3; i++ {
		got := p.Plan(screen, []Damage{{screen, UI}})
		full := got[0].Full
		if (i == 3) != full {
			t.Fatalf("крупное обновление %d: full=%v", i, full)
		}
	}
	small := p.Plan(screen, []Damage{{image.Rect(0, 0, 100, 100), UI}})
	if small[0].Full {
		t.Fatal("мелкое обновление не должно мигать")
	}
}

func TestMerge(t *testing.T) {
	got := Merge([]image.Rectangle{
		image.Rect(0, 0, 10, 10),
		image.Rect(10, 0, 20, 10), // соседний
		image.Rect(500, 500, 510, 510),
		image.Rect(505, 505, 520, 520), // пересекается
		{},
	})
	want := []image.Rectangle{image.Rect(0, 0, 20, 10), image.Rect(500, 500, 520, 520)}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Merge = %v, ожидалось %v", got, want)
	}
}

type fakeDisplay struct {
	presented []device.Update
	waited    []device.Marker
	next      device.Marker
}

func (f *fakeDisplay) Bounds() image.Rectangle           { return screen }
func (f *fakeDisplay) DPI() int                          { return 300 }
func (f *fakeDisplay) Rotation() device.Rotation         { return device.Portrait }
func (f *fakeDisplay) SetRotation(device.Rotation) error { return nil }
func (f *fakeDisplay) Wait(m device.Marker) error        { f.waited = append(f.waited, m); return nil }
func (f *fakeDisplay) Present(_ *image.Gray, u device.Update) (device.Marker, error) {
	f.next++
	f.presented = append(f.presented, u)
	return f.next, nil
}

func TestSchedulerWaitsOnlyForConflicts(t *testing.T) {
	d := &fakeDisplay{}
	s := NewScheduler(d, nil)
	clock := time.Unix(0, 0)
	s.now = func() time.Time { return clock }
	img := image.NewGray(screen)

	stroke := func(x int) device.Update {
		return device.Update{Rect: image.Rect(x, 0, x+20, 20), Waveform: device.WaveformDU}
	}
	_ = s.Present(img, stroke(0))
	_ = s.Present(img, stroke(10)) // пересекается, тот же режим — без ожидания
	if len(d.waited) != 0 {
		t.Fatalf("штрихи одним режимом не должны ждать: %v", d.waited)
	}
	_ = s.Present(img, device.Update{Rect: image.Rect(0, 0, 100, 100), Waveform: device.WaveformGL16})
	if len(d.waited) != 2 {
		t.Fatalf("GL16 поверх двух DU должен дождаться обоих: %v", d.waited)
	}
	_ = s.Present(img, device.Update{Rect: image.Rect(1000, 1000, 1100, 1100), Waveform: device.WaveformGC16})
	if len(d.waited) != 2 {
		t.Fatalf("непересекающееся обновление не должно ждать: %v", d.waited)
	}
	clock = clock.Add(time.Second)
	_ = s.Present(img, device.Update{Rect: screen, Waveform: device.WaveformGC16, Full: true})
	if len(d.waited) != 2 {
		t.Fatalf("закончившиеся по времени обновления не ждут: %v", d.waited)
	}
}
