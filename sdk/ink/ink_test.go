package ink

import (
	"bytes"
	"compress/zlib"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"math/rand"
	"strings"
	"testing"
	"time"

	"el-book/sdk/gfx"
)

func newSheet(w, h int) (*image.Gray, *gfx.Canvas) {
	img := image.NewGray(image.Rect(0, 0, w, h))
	c := gfx.NewCanvas(img)
	c.Fill(img.Bounds(), gfx.White)
	return img, c
}

// TestCapsuleMatchesBruteForce сверяет быстрый обход строк с честным обходом всего
// охватывающего прямоугольника: ни одна точка капсулы не должна потеряться.
func TestCapsuleMatchesBruteForce(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for n := 0; n < 300; n++ {
		// Явное приведение запрещает компилятору сливать умножение с вычитанием (FMA):
		// иначе «нулевой» отрезок получает длину 1e-14.
		x0, y0 := float64(rng.Float64()*200), float64(rng.Float64()*200)
		x1, y1 := float64(rng.Float64()*200), float64(rng.Float64()*200)
		if n%7 == 0 {
			x1 = x0 // вертикальный
		}
		if n%11 == 0 {
			y1 = y0 // горизонтальный
		}
		r0, r1 := 0.5+rng.Float64()*8, 0.5+rng.Float64()*8
		fast, c := newSheet(220, 220)
		newSpan(c, gfx.Black, normal, Fine).capsule(x0, y0, x1, y1, r0, r1)

		slow, _ := newSheet(220, 220)
		dx, dy := x1-x0, y1-y0
		ll := dx*dx + dy*dy
		for y := 0; y < 220; y++ {
			for x := 0; x < 220; x++ {
				px, py := float64(x)+0.5, float64(y)+0.5
				tt := 0.0
				if ll > 0 {
					tt = math.Max(0, math.Min(1, ((px-x0)*dx+(py-y0)*dy)/ll))
				}
				d := math.Hypot(px-(x0+tt*dx), py-(y0+tt*dy))
				cov := r0 + tt*(r1-r0) + 0.5 - d
				if cov <= 0 {
					continue
				}
				a := uint32(255)
				if cov < 1 {
					a = uint32(cov * 255)
				}
				slow.Pix[slow.PixOffset(x, y)] = uint8((255*(255-a) + 127) / 255)
			}
		}
		if !bytes.Equal(fast.Pix, slow.Pix) {
			t.Fatalf("капсула %d (%.1f,%.1f)–(%.1f,%.1f) r %.1f…%.1f: обход строк потерял точки", n, x0, y0, x1, y1, r0, r1)
		}
	}
}

func scribble(seed int64, brush Brush, n int) *Stroke {
	rng := rand.New(rand.NewSource(seed))
	s := &Stroke{Brush: brush, Width: 6, Tone: gfx.Black, Seed: uint32(seed)}
	x, y := 150.0, 150.0
	for i := 0; i < n; i++ {
		x += rng.Float64()*20 - 10
		y += rng.Float64()*20 - 10
		s.Pts = append(s.Pts, Point{X: float32(x), Y: float32(y), P: float32(0.4 + rng.Float64())})
	}
	return s
}

func TestIncrementalEqualsFull(t *testing.T) {
	for _, b := range append(Brushes, Eraser) {
		s := scribble(int64(b)+3, b, 80)
		if b == Eraser {
			s.Tone = gfx.White
		}
		for _, q := range []Quality{Fast, Fine} {
			full, fc := newSheet(300, 300)
			s.Draw(fc, q)
			inc, ic := newSheet(300, 300)
			box := image.Rectangle{}
			for i := range s.Pts {
				box = box.Union(s.DrawPoint(ic, i, q))
			}
			if !bytes.Equal(full.Pix, inc.Pix) {
				t.Errorf("%s, качество %d: по точкам и целиком рисуется по-разному", b.Title(), q)
			}
			if outside := inkOutside(full, s.Bounds()); outside > 0 {
				t.Errorf("%s: %d точек краски за пределами Bounds %v", b.Title(), outside, s.Bounds())
			}
			if !box.Empty() && !s.Bounds().Union(box).Eq(s.Bounds().Union(box)) {
				t.Errorf("%s: странная область", b.Title())
			}
		}
	}
}

func inkOutside(img *image.Gray, r image.Rectangle) int {
	n := 0
	for y := 0; y < img.Rect.Dy(); y++ {
		for x := 0; x < img.Rect.Dx(); x++ {
			if img.Pix[img.PixOffset(x, y)] != gfx.White && !image.Pt(x, y).In(r) {
				n++
			}
		}
	}
	return n
}

func TestFastIsTwoTone(t *testing.T) {
	s := scribble(5, Marker, 40)
	s.Tone = gfx.Gray16(9)
	img, c := newSheet(300, 300)
	s.Draw(c, Fast)
	for _, v := range img.Pix {
		if v != gfx.Black && v != gfx.White {
			t.Fatalf("быстрый режим дал серый %d", v)
		}
	}
}

func TestMarkerDoesNotCoverInk(t *testing.T) {
	img, c := newSheet(100, 100)
	c.Fill(image.Rect(40, 0, 60, 100), gfx.Black)
	m := &Stroke{Brush: Marker, Width: 30, Tone: gfx.Gray16(10), Pts: []Point{{X: 5, Y: 50, P: 1}, {X: 95, Y: 50, P: 1}}}
	m.Draw(c, Fine)
	if v := img.Pix[img.PixOffset(50, 50)]; v != gfx.Black {
		t.Errorf("маркер осветлил чёрное: %d", v)
	}
	if v := img.Pix[img.PixOffset(20, 50)]; v != gfx.Gray16(10) {
		t.Errorf("маркер на белом: %d, ждали %d", v, gfx.Gray16(10))
	}
}

func TestCodecRoundTrip(t *testing.T) {
	var in []*Stroke
	for i, b := range append(Brushes, Eraser) {
		s := scribble(int64(i), b, 50+i)
		s.Width = 3.25 + float32(i)
		s.Tone = uint8(i * 30)
		in = append(in, s)
	}
	data := Marshal(in)
	out, err := Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(in) {
		t.Fatalf("прочитано %d штрихов из %d", len(out), len(in))
	}
	for i := range in {
		a, b := in[i], out[i]
		if a.Brush != b.Brush || a.Tone != b.Tone || a.Seed != b.Seed || a.Width != b.Width || len(a.Pts) != len(b.Pts) {
			t.Fatalf("штрих %d: %+v → %+v", i, a, b)
		}
		for j := range a.Pts {
			if math.Abs(float64(a.Pts[j].X-b.Pts[j].X)) > 1.0/16 || math.Abs(float64(a.Pts[j].Y-b.Pts[j].Y)) > 1.0/16 {
				t.Fatalf("штрих %d, точка %d: %v → %v", i, j, a.Pts[j], b.Pts[j])
			}
			if a.Brush.Dynamic() && math.Abs(float64(a.Pts[j].P-b.Pts[j].P)) > 0.01 {
				t.Fatalf("штрих %d, точка %d: толщина %v → %v", i, j, a.Pts[j].P, b.Pts[j].P)
			}
		}
	}
	// Формат заметно меньше JSON с целыми координатами.
	if per := float64(len(data)) / float64(countPoints(in)); per > 4.5 {
		t.Errorf("%.1f байта на точку — формат раздулся", per)
	}
	for _, bad := range [][]byte{nil, []byte("PNG"), data[:len(data)/2], append([]byte("INK\x09"), data[4:]...)} {
		if _, err := Unmarshal(bad); !errors.Is(err, ErrFormat) {
			t.Errorf("испорченные данные %q…: %v", bad[:min(len(bad), 6)], err)
		}
	}
}

func countPoints(ss []*Stroke) int {
	n := 0
	for _, s := range ss {
		n += len(s.Pts)
	}
	return n
}

// trace — точки по контуру с дрожанием руки.
func trace(seed int64, f func(t float64) (float64, float64), n int, jitter float64) []Point {
	rng := rand.New(rand.NewSource(seed))
	out := make([]Point, 0, n)
	for i := 0; i < n; i++ {
		x, y := f(float64(i) / float64(n-1))
		out = append(out, Point{X: float32(x + rng.NormFloat64()*jitter), Y: float32(y + rng.NormFloat64()*jitter), P: 1})
	}
	return out
}

func polygon(corners ...[2]float64) func(t float64) (float64, float64) {
	return func(t float64) (float64, float64) {
		n := len(corners)
		pos := t * float64(n)
		i := min(int(pos), n-1)
		f := pos - float64(i)
		a, b := corners[i], corners[(i+1)%n]
		return a[0] + (b[0]-a[0])*f, a[1] + (b[1]-a[1])*f
	}
}

func TestRecognize(t *testing.T) {
	cases := []struct {
		name string
		pts  []Point
		want Shape
	}{
		{"линия", trace(1, func(t float64) (float64, float64) { return 100 + 600*t, 200 + 20*t }, 60, 2), Line},
		{"прямоугольник", trace(2, polygon([2]float64{100, 100}, [2]float64{700, 110}, [2]float64{690, 500}, [2]float64{95, 495}), 160, 3), Rectangle},
		{"треугольник", trace(3, polygon([2]float64{400, 100}, [2]float64{700, 600}, [2]float64{100, 600}), 150, 3), Triangle},
		{"окружность", trace(4, func(t float64) (float64, float64) {
			return 400 + 250*math.Cos(2*math.Pi*t), 400 + 240*math.Sin(2*math.Pi*t)
		}, 120, 4), Ellipse},
		{"каракуля", scribble(9, Pen, 120).Pts, NoShape},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, pts := Recognize(c.pts, 60)
			if got != c.want {
				t.Fatalf("узнано %q, ждали %q", got.Title(), c.want.Title())
			}
			if got == Line && pts[0].Y != pts[1].Y && math.Abs(float64(pts[1].Y-pts[0].Y)) > 30 {
				t.Errorf("почти горизонтальная линия не выровнена: %v", pts)
			}
			if got == Rectangle && (pts[0].Y != pts[1].Y || pts[1].X != pts[2].X) {
				t.Errorf("прямоугольник не выровнен по осям: %v", pts)
			}
		})
	}
	if got, _ := Recognize(trace(5, func(t float64) (float64, float64) { return 10 + 20*t, 10 }, 10, 0), 60); got != NoShape {
		t.Errorf("короткий росчерк узнан как %q", got.Title())
	}
}

func TestHitAndLasso(t *testing.T) {
	s := &Stroke{Brush: Pen, Width: 10, Pts: []Point{{X: 100, Y: 100, P: 1}, {X: 300, Y: 100, P: 1}}}
	if !s.Hit(200, 108, 4) || s.Hit(200, 130, 4) {
		t.Error("попадание в штрих считается неверно")
	}
	l := Lasso{{X: 50, Y: 50}, {X: 350, Y: 50}, {X: 350, Y: 150}, {X: 50, Y: 150}}
	if !l.Selects(s) {
		t.Error("лассо не выделило штрих внутри")
	}
	half := Lasso{{X: 50, Y: 50}, {X: 150, Y: 50}, {X: 150, Y: 150}, {X: 50, Y: 150}}
	if half.Selects(&Stroke{Width: 2, Pts: []Point{{X: 100, Y: 100}, {X: 200, Y: 100}, {X: 300, Y: 100}}}) {
		t.Error("лассо выделило штрих, который почти весь снаружи")
	}
}

func TestSmoother(t *testing.T) {
	var s Smoother
	start := time.Unix(0, 0)
	s.Begin(0, 0, start)
	if _, ok := s.Move(1, 0, start.Add(8*time.Millisecond)); ok {
		t.Error("точка ближе MinStep не отброшена")
	}
	pen := Smoother{Speed: true}
	pen.Begin(0, 0, start)
	var slow, fast float32
	for i := 1; i <= 20; i++ {
		p, ok := pen.Move(float64(i)*3, 0, start.Add(time.Duration(i)*16*time.Millisecond))
		if ok {
			slow = p.P
		}
	}
	for i := 1; i <= 20; i++ {
		p, ok := pen.Move(60+float64(i)*60, 0, start.Add(time.Duration(320+i*16)*time.Millisecond))
		if ok {
			fast = p.P
		}
	}
	if !(slow > 1.1 && fast < 0.6) {
		t.Errorf("толщина по скорости: медленно %.2f, быстро %.2f", slow, fast)
	}
}

func BenchmarkLongDiagonal(b *testing.B) {
	_, c := newSheet(1440, 1800)
	sp := newSpan(c, gfx.Black, normal, Fine)
	for i := 0; i < b.N; i++ {
		sp.capsule(10, 10, 1400, 1750, 3, 3)
	}
}

func BenchmarkPageRender(b *testing.B) {
	strokes := make([]*Stroke, 0, 300)
	for i := 0; i < 300; i++ {
		strokes = append(strokes, scribble(int64(i), Pen, 60))
	}
	_, c := newSheet(1440, 1800)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, s := range strokes {
			s.Draw(c, Fine)
		}
	}
}

func testSheet() Sheet {
	return Sheet{Width: 400, Height: 300,
		Strokes: []*Stroke{scribble(1, Pen, 30), scribble(2, Fountain, 30), scribble(3, Sketch, 30)},
		Ruling:  []Seg{{X0: 0, Y0: 100, X1: 400, Y1: 100, R0: 1, R1: 1, Tone: 200}},
	}
}

func TestWriteSVG(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteSVG(&buf, testSheet()); err != nil {
		t.Fatal(err)
	}
	dec := xml.NewDecoder(&buf)
	paths := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("SVG не разбирается: %v", err)
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "path" {
			paths++
		}
	}
	// Ручка — один путь, перо — несколько по толщине, набросок — линия и связи, разметка.
	if paths < 4 || paths > 40 {
		t.Errorf("путей в SVG: %d", paths)
	}
}

func TestWritePDF(t *testing.T) {
	var buf bytes.Buffer
	if err := WritePDF(&buf, []Sheet{testSheet(), testSheet()}, 300); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	if !bytes.HasPrefix(data, []byte("%PDF-1.4")) || !bytes.HasSuffix(data, []byte("%%EOF\n")) {
		t.Fatal("не похоже на PDF")
	}
	// Таблица xref указывает точно на начала объектов.
	xrefAt := bytes.LastIndex(data, []byte("startxref\n"))
	var xref int
	fmt.Sscanf(string(data[xrefAt+len("startxref\n"):]), "%d", &xref)
	lines := strings.Split(string(data[xref:]), "\n")
	var count int
	fmt.Sscanf(lines[1], "0 %d", &count)
	for n := 1; n < count; n++ {
		var off int
		fmt.Sscanf(lines[2+n], "%d", &off)
		if want := fmt.Sprintf("%d 0 obj", n); !bytes.HasPrefix(data[off:], []byte(want)) {
			t.Fatalf("xref: объект %d по смещению %d не найден", n, off)
		}
	}
	if c := bytes.Count(data, []byte("/Type /Page ")); c != 2 {
		t.Errorf("страниц %d, ждали 2", c)
	}
	// Поток страницы распаковывается и рисует линии.
	start := bytes.Index(data, []byte("stream\n")) + len("stream\n")
	end := bytes.Index(data[start:], []byte("\nendstream"))
	zr, err := zlib.NewReader(bytes.NewReader(data[start : start+end]))
	if err != nil {
		t.Fatal(err)
	}
	content, _ := io.ReadAll(zr)
	if !bytes.Contains(content, []byte(" l S\n")) || !bytes.Contains(content, []byte("/D gs")) {
		t.Errorf("в потоке страницы нет линий или затемнения:\n%.300s", content)
	}
}
