package draw

import (
	"encoding/json"
	"image"
	"testing"
	"time"

	"el-book/sdk/gfx"
	"el-book/sdk/ink"
	"el-book/sdk/storage"
)

func testStore(t *testing.T) *storage.App {
	t.Helper()
	s, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s.App("draw")
}

func TestBookRoundTrip(t *testing.T) {
	st := testStore(t)
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	b := newBook(now, 600, 800, 300, Grid)
	b.Title = "Скетчи"
	b.Pages[0].strokes = []*ink.Stroke{{Brush: ink.Fountain, Width: 6, Pts: []ink.Point{{X: 10, Y: 10, P: 1}, {X: 200, Y: 300, P: 0.5}}}}
	n := b.addPage(now.Add(time.Second), 0, Oblique)
	b.Pages[n].strokes = []*ink.Stroke{{Brush: ink.Marker, Width: 40, Tone: 150, Pts: []ink.Point{{X: 50, Y: 50, P: 1}}}}
	b.Current = 1
	layer := image.NewGray(image.Rect(0, 0, 600, 800))
	snap, err := b.snapshot(layer, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := snap.write(st); err != nil {
		t.Fatal(err)
	}
	if b.Pages[0].dirty || b.Pages[1].dirty {
		t.Error("после снимка страницы всё ещё отмечены изменёнными")
	}

	back, err := loadBook(st, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if back.Title != "Скетчи" || back.Current != 1 || len(back.Pages) != 2 || back.template(back.Pages[1]) != Oblique || back.template(back.Pages[0]) != Grid {
		t.Fatalf("тетрадь прочиталась не так: %+v", back)
	}
	for i, p := range back.Pages {
		if err := back.load(st, p); err != nil {
			t.Fatal(err)
		}
		if len(p.strokes) != 1 {
			t.Fatalf("страница %d: %d штрихов", i, len(p.strokes))
		}
	}
	if back.Pages[1].strokes[0].Brush != ink.Marker || back.Pages[0].strokes[0].Pts[1].P != 0.5 {
		t.Error("штрихи прочитались не так")
	}
	if !st.Exists(bookThumb(b.ID)) {
		t.Error("нет миниатюры")
	}
}

func TestMigrateV1(t *testing.T) {
	st := testStore(t)
	v1 := map[string]any{
		"version": 1, "id": "abc", "created": 1760000000, "modified": 1760000100,
		"width": 600, "height": 800, "background": "lines",
		"strokes": []map[string]any{
			{"w": 10.0, "c": 0, "p": []int32{10, 10, 100, 100, 200, 120}},
			{"w": 40.0, "c": 255, "p": []int32{50, 50, 60, 60}},
		},
	}
	data, _ := json.Marshal(v1)
	if err := st.WriteFile("drawings/abc.json", data); err != nil {
		t.Fatal(err)
	}
	if err := st.WriteFile("drawings/abc.png", []byte("png")); err != nil {
		t.Fatal(err)
	}
	n, err := migrateV1(st)
	if err != nil || n != 1 {
		t.Fatalf("перенесено %d, ошибка %v", n, err)
	}
	if st.Exists("drawings/abc.json") || st.Exists("drawings") {
		t.Error("старый рисунок остался")
	}
	b, err := loadBook(st, "abc")
	if err != nil {
		t.Fatal(err)
	}
	if b.Template != Lines || b.Width != 600 || len(b.Pages) != 1 {
		t.Fatalf("тетрадь после переноса: %+v", b)
	}
	if err := b.load(st, b.Pages[0]); err != nil {
		t.Fatal(err)
	}
	s := b.Pages[0].strokes
	if len(s) != 2 || s[0].Brush != ink.Pen || s[1].Brush != ink.Eraser || len(s[0].Pts) != 3 || s[0].Pts[2].X != 200.5 {
		t.Errorf("штрихи после переноса: %+v %+v", s[0], s[1])
	}
	// Повторный запуск ничего не ломает.
	if n, err := migrateV1(st); n != 0 || err != nil {
		t.Errorf("повторный перенос: %d, %v", n, err)
	}
}

// TestEraserRestoresRuling: ластик стирает штрихи, а разметка листа видна снова.
func TestEraserRestoresRuling(t *testing.T) {
	layer := image.NewGray(image.Rect(0, 0, 400, 400))
	rule := ruling(Grid, 400, 400, 300)
	strokes := []*ink.Stroke{
		{Brush: ink.Pen, Width: 80, Tone: gfx.Black, Pts: []ink.Point{{X: 50, Y: 59, P: 1}, {X: 350, Y: 59, P: 1}}},
		{Brush: ink.Eraser, Width: 100, Tone: gfx.White, Pts: []ink.Point{{X: 200, Y: 59, P: 1}}},
	}
	render(layer, layer.Bounds(), strokes, rule)
	gridLine := layer.GrayAt(200, 59).Y // линия клетки на 5 мм под стёртым местом
	if gridLine == gfx.White || gridLine == gfx.Black {
		t.Errorf("под ластиком линия клетки %d: ждали серую", gridLine)
	}
	if v := layer.GrayAt(80, 59).Y; v != gfx.Black {
		t.Errorf("нестёртая часть штриха %d", v)
	}
	if v := layer.GrayAt(200, 30).Y; v != gfx.White {
		t.Errorf("стёртое место без линии %d", v)
	}
}

func TestTemplatesDrawSomething(t *testing.T) {
	for _, tm := range templates {
		rule := ruling(tm, 1440, 1780, 300)
		if (tm == Plain) != (len(rule) == 0) {
			t.Errorf("%s: %d линий", tm.title(), len(rule))
		}
	}
}

func TestPageTemplateIsOwn(t *testing.T) {
	st := testStore(t)
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	b := newBook(now, 600, 800, 300, Lines)
	n := b.addPage(now.Add(time.Second), 0, b.template(b.Pages[0]))
	b.Pages[n].Template = Grid
	b.Template = Grid // панель страниц запоминает выбор для новых страниц
	if got := b.template(b.Pages[0]); got != Lines {
		t.Fatalf("первая страница сменила разметку вслед за второй: %v", got)
	}

	// Тетрадь, записанная до того, как у страниц появилась своя разметка.
	old := Book{Version: bookVersion, ID: "old", Created: now.Unix(), Width: 600, Height: 800, DPI: 300,
		Template: Dots, Pages: []*PageRef{{ID: "p1"}}}
	data, _ := json.Marshal(old)
	if err := st.WriteFile(bookFile("old"), data); err != nil {
		t.Fatal(err)
	}
	back, err := loadBook(st, "old")
	if err != nil {
		t.Fatal(err)
	}
	back.Template = Music
	if got := back.template(back.Pages[0]); got != Dots {
		t.Fatalf("страница старой тетради должна сохранить разметку тетради, а не новую: %v", got)
	}
}
