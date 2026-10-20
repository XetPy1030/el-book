package draw

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"strconv"
	"strings"
	"time"

	"el-book/sdk/gfx"
	"el-book/sdk/ink"
	"el-book/sdk/storage"
)

// bookVersion — версия формата тетради. Версия 1 — одиночный рисунок drawings/<id>.json,
// он переносится в тетрадь при первом открытии галереи (migrateV1).
const bookVersion = 2

// Book — тетрадь: страницы со штрихами и разметка листа.
//
//	books/<id>/book.json   — описание и порядок страниц
//	books/<id>/<стр>.ink   — штрихи страницы (sdk/ink)
//	books/<id>/thumb.png   — миниатюра открытой страницы для галереи
type Book struct {
	Version  int        `json:"version"`
	ID       string     `json:"id"`
	Title    string     `json:"title"`
	Created  int64      `json:"created"`
	Modified int64      `json:"modified"`
	Width    int        `json:"width"`
	Height   int        `json:"height"`
	DPI      int        `json:"dpi"`
	Template Template   `json:"template"`
	Pages    []*PageRef `json:"pages"`
	Current  int        `json:"current"` // открытая страница
}

// PageRef — страница тетради. Штрихи читаются при первом показе страницы.
type PageRef struct {
	ID       string   `json:"id"`
	Template Template `json:"template,omitempty"` // разметка; пусто (у старых тетрадей) — как у тетради

	strokes []*ink.Stroke
	loaded  bool
	dirty   bool
}

// template — разметка страницы.
func (b *Book) template(p *PageRef) Template {
	if p.Template != "" {
		return p.Template
	}
	if b.Template == "" {
		return Plain
	}
	return b.Template
}

func newID(now time.Time) string { return strconv.FormatInt(now.UnixNano(), 36) }

// newBook создаёт тетрадь с одной пустой страницей размером w×h. Разметка тетради —
// для новых страниц; у каждой страницы своя, чтобы смена разметки одной страницы не
// трогала остальные.
func newBook(now time.Time, w, h, dpi int, t Template) *Book {
	b := &Book{Version: bookVersion, ID: newID(now), Created: now.Unix(), Modified: now.Unix(),
		Width: w, Height: h, DPI: dpi, Template: t}
	b.Pages = []*PageRef{{ID: newID(now.Add(time.Nanosecond)), Template: t, loaded: true, dirty: true}}
	return b
}

// addPage вставляет пустую страницу после at и возвращает её номер.
func (b *Book) addPage(now time.Time, at int, t Template) int {
	p := &PageRef{ID: newID(now), Template: t, loaded: true, dirty: true}
	at = min(max(at+1, 0), len(b.Pages))
	b.Pages = append(b.Pages[:at], append([]*PageRef{p}, b.Pages[at:]...)...)
	return at
}

func (b *Book) dir() string               { return "books/" + b.ID }
func bookFile(id string) string           { return "books/" + id + "/book.json" }
func bookThumb(id string) string          { return "books/" + id + "/thumb.png" }
func pageFile(id string, p string) string { return "books/" + id + "/" + p + ".ink" }

// title — название для галереи: своё или дата создания.
func (b *Book) title() string {
	if strings.TrimSpace(b.Title) != "" {
		return b.Title
	}
	return dateTitle(time.Unix(b.Created, 0))
}

// load читает штрихи страницы, если они ещё не прочитаны.
func (b *Book) load(st *storage.App, p *PageRef) error {
	if p.loaded {
		return nil
	}
	data, err := st.ReadFile(pageFile(b.ID, p.ID))
	if errors.Is(err, fs.ErrNotExist) {
		p.loaded = true // страницу создали и не успели записать
		return nil
	}
	if err != nil {
		return err
	}
	strokes, err := ink.Unmarshal(data)
	if err != nil {
		return fmt.Errorf("страница %s: %w", p.ID, err)
	}
	p.strokes, p.loaded = strokes, true
	return nil
}

// loadBook читает описание тетради; страницы читаются по мере показа.
func loadBook(st *storage.App, id string) (*Book, error) {
	data, err := st.ReadFile(bookFile(id))
	if err != nil {
		return nil, err
	}
	var b Book
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("тетрадь %s повреждена: %w", id, err)
	}
	if b.Version > bookVersion {
		return nil, fmt.Errorf("тетрадь %s записана более новой версией (%d)", id, b.Version)
	}
	if len(b.Pages) == 0 {
		b.Pages = []*PageRef{{ID: newID(time.Unix(b.Created, 0)), loaded: true}}
	}
	b.Current = min(max(b.Current, 0), len(b.Pages)-1)
	if b.DPI == 0 {
		b.DPI = 300
	}
	for _, p := range b.Pages {
		p.Template = b.template(p) // страницы без своей разметки получают разметку тетради
	}
	return &b, nil
}

// bookSnapshot — тетрадь, готовая к записи: данные сняты в цикле, писать можно из фона.
type bookSnapshot struct {
	id    string
	meta  []byte
	pages map[string][]byte
	thumb *image.Gray
	now   time.Time
}

// snapshot снимает описание, изменённые страницы и миниатюру слоя открытой страницы.
func (b *Book) snapshot(layer *image.Gray, now time.Time) (bookSnapshot, error) {
	b.Modified = now.Unix()
	meta, err := json.Marshal(b)
	if err != nil {
		return bookSnapshot{}, fmt.Errorf("тетрадь %s: %w", b.ID, err)
	}
	snap := bookSnapshot{id: b.ID, meta: meta, pages: map[string][]byte{}, now: now}
	for _, p := range b.Pages {
		if p.dirty {
			snap.pages[p.ID] = ink.Marshal(p.strokes)
			p.dirty = false
		}
	}
	if layer != nil {
		snap.thumb = gfx.Downscale(layer, max(b.Width/thumbScale, 1), max(b.Height/thumbScale, 1))
	}
	return snap, nil
}

// thumbScale — во сколько раз миниатюра меньше страницы.
const thumbScale = 5

// write записывает снимок: сначала страницы, потом описание — описание не ссылается на
// незаписанную страницу.
func (s bookSnapshot) write(st *storage.App) error {
	for id, data := range s.pages {
		if err := st.WriteFile(pageFile(s.id, id), data); err != nil {
			return err
		}
	}
	if s.thumb != nil {
		var buf bytes.Buffer
		if err := png.Encode(&buf, s.thumb); err != nil {
			return fmt.Errorf("миниатюра %s: %w", s.id, err)
		}
		if err := st.WriteFile(bookThumb(s.id), buf.Bytes()); err != nil {
			return err
		}
	}
	if err := st.WriteFile(bookFile(s.id), s.meta); err != nil {
		return err
	}
	return st.SetModTime(bookFile(s.id), s.now)
}

// removePage удаляет файл страницы, которой больше нет в тетради.
func removePage(st *storage.App, bookID, pageID string) {
	_ = st.Remove(pageFile(bookID, pageID))
}

// render перерисовывает область r слоя страницы: белый лист, штрихи и разметка поверх
// затемнением. extra — штрихи, которые ещё ведут пальцы.
func render(layer *image.Gray, r image.Rectangle, strokes []*ink.Stroke, rule []ink.Seg, extra ...*ink.Stroke) {
	r = r.Intersect(layer.Bounds())
	if r.Empty() {
		return
	}
	c := gfx.NewCanvas(layer).WithClip(r)
	c.Fill(r, gfx.White)
	for _, s := range strokes {
		if s.Bounds().Overlaps(r) {
			s.Draw(c, ink.Fine)
		}
	}
	for _, s := range extra {
		if s.Bounds().Overlaps(r) {
			s.Draw(c, ink.Fast)
		}
	}
	drawRuling(c, rule)
}

// drawRuling кладёт разметку на холст затемнением.
func drawRuling(c *gfx.Canvas, rule []ink.Seg) {
	for _, g := range rule {
		ink.DrawSeg(c, g, ink.Fine)
	}
}

// v1Doc — рисунок первой версии.
type v1Doc struct {
	Version    int    `json:"version"`
	ID         string `json:"id"`
	Created    int64  `json:"created"`
	Modified   int64  `json:"modified"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	Background string `json:"background"`
	Strokes    []struct {
		Width float64 `json:"w"`
		Color uint8   `json:"c"`
		Pts   []int32 `json:"p"`
	} `json:"strokes"`
}

// migrateV1 переносит рисунки первой версии (drawings/*.json) в тетради. Старый файл
// удаляется только после того, как тетрадь записана и прочитана обратно. Возвращает,
// сколько рисунков перенесено.
func migrateV1(st *storage.App) (int, error) {
	files, err := st.List("drawings")
	if err != nil || len(files) == 0 {
		return 0, err
	}
	n := 0
	var errs []error
	for _, f := range files {
		id, ok := strings.CutSuffix(f.Name, ".json")
		if !ok {
			continue
		}
		if err := migrateOne(st, id, time.Unix(f.ModTime, 0)); err != nil {
			errs = append(errs, fmt.Errorf("рисунок %s: %w", id, err))
			continue
		}
		n++
	}
	if left, _ := st.List("drawings"); len(left) == 0 {
		_ = st.Remove("drawings")
	}
	return n, errors.Join(errs...)
}

func migrateOne(st *storage.App, id string, modified time.Time) error {
	data, err := st.ReadFile("drawings/" + id + ".json")
	if err != nil {
		return err
	}
	var d v1Doc
	if err := json.Unmarshal(data, &d); err != nil {
		return err
	}
	b := &Book{Version: bookVersion, ID: id, Created: d.Created, Modified: d.Modified,
		Width: d.Width, Height: d.Height, DPI: 300, Template: Template(d.Background)}
	if b.Template == "" {
		b.Template = Plain
	}
	page := &PageRef{ID: "p1", loaded: true, dirty: true}
	for _, s := range d.Strokes {
		st := &ink.Stroke{Brush: ink.Pen, Width: float32(s.Width), Tone: s.Color}
		if s.Color == gfx.White {
			st.Brush = ink.Eraser
		}
		for i := 0; i+1 < len(s.Pts); i += 2 {
			st.Pts = append(st.Pts, ink.Point{X: float32(s.Pts[i]) + 0.5, Y: float32(s.Pts[i+1]) + 0.5, P: 1})
		}
		page.strokes = append(page.strokes, st)
	}
	b.Pages = []*PageRef{page}
	snap, err := b.snapshot(nil, modified)
	if err != nil {
		return err
	}
	if err := snap.write(st); err != nil {
		return err
	}
	if thumb, err := st.ReadFile("drawings/" + id + ".png"); err == nil {
		_ = st.WriteFile(bookThumb(id), thumb)
	}
	// Проверка: тетрадь читается, штрихов столько же.
	back, err := loadBook(st, id)
	if err != nil {
		return err
	}
	if err := back.load(st, back.Pages[0]); err != nil {
		return err
	}
	if len(back.Pages[0].strokes) != len(d.Strokes) {
		return fmt.Errorf("после переноса %d штрихов из %d", len(back.Pages[0].strokes), len(d.Strokes))
	}
	_ = st.Remove("drawings/" + id + ".json")
	_ = st.Remove("drawings/" + id + ".png")
	return nil
}
