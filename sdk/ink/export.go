package ink

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"math"
	"strconv"
)

// Sheet — лист для выгрузки: размер в пикселях, штрихи и разметка.
type Sheet struct {
	Width, Height int
	Strokes       []*Stroke
	// Ruling — линии разметки: ложатся поверх штрихов затемнением, как на экране, поэтому
	// стёртое ластиком место снова показывает линейку.
	Ruling []Seg
}

// path — отрезки одной толщины и тона, слитые в ломаные: так выгрузка в разы меньше,
// чем по линии на отрезок.
type path struct {
	tone   uint8
	darken bool
	width  float32
	parts  [][]float32 // ломаные: x0 y0 x1 y1 …
}

// paths сводит отрезки листа в ломаные: сначала штрихи, затем разметка.
func (sh Sheet) paths() []path {
	var out []path
	var cur *path
	add := func(g Seg) {
		w := quantize(g.R0 + g.R1)
		if w <= 0 {
			return
		}
		if cur == nil || cur.tone != g.Tone || cur.darken != g.Darken || cur.width != w {
			out = append(out, path{tone: g.Tone, darken: g.Darken, width: w})
			cur = &out[len(out)-1]
		}
		if n := len(cur.parts); n > 0 {
			last := cur.parts[n-1]
			if last[len(last)-2] == g.X0 && last[len(last)-1] == g.Y0 {
				cur.parts[n-1] = append(last, g.X1, g.Y1)
				return
			}
		}
		cur.parts = append(cur.parts, []float32{g.X0, g.Y0, g.X1, g.Y1})
	}
	for _, s := range sh.Strokes {
		var links []Seg
		for i := range s.Pts {
			first := true
			s.Segs(i, func(g Seg) {
				if first {
					add(g)
					first = false
					return
				}
				links = append(links, g)
			})
		}
		// Связи процедурных кистей — после основной линии одним путём.
		for _, g := range links {
			add(g)
		}
		cur = nil
	}
	for _, g := range sh.Ruling {
		g.Darken = true
		add(g)
	}
	return out
}

// quantize округляет толщину до четверти пикселя: перо с плавной толщиной даёт меньше путей.
func quantize(w float32) float32 { return float32(math.Round(float64(w)*4) / 4) }

func num(v float32) string { return strconv.FormatFloat(math.Round(float64(v)*10)/10, 'f', -1, 64) }

// WriteSVG выгружает лист в SVG.
func WriteSVG(w io.Writer, sh Sheet) error {
	bw := bufio.NewWriter(w)
	fmt.Fprintf(bw, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`+"\n",
		sh.Width, sh.Height, sh.Width, sh.Height)
	fmt.Fprintf(bw, `<rect width="%d" height="%d" fill="#fff"/>`+"\n", sh.Width, sh.Height)
	for _, p := range sh.paths() {
		fmt.Fprintf(bw, `<path fill="none" stroke="#%02x%02x%02x" stroke-width="%s" stroke-linecap="round" stroke-linejoin="round"`,
			p.tone, p.tone, p.tone, num(p.width))
		if p.darken {
			bw.WriteString(` style="mix-blend-mode:darken"`)
		}
		bw.WriteString(` d="`)
		for _, part := range p.parts {
			bw.WriteString("M" + num(part[0]) + " " + num(part[1]))
			for k := 2; k < len(part); k += 2 {
				bw.WriteString("L" + num(part[k]) + " " + num(part[k+1]))
			}
		}
		bw.WriteString("\"/>\n")
	}
	bw.WriteString("</svg>\n")
	return bw.Flush()
}

// WritePDF выгружает листы в векторный PDF, страница на лист. dpi — плотность, в которой
// заданы пиксели листа: по ней считается размер страницы.
func WritePDF(w io.Writer, sheets []Sheet, dpi int) error {
	if dpi <= 0 {
		dpi = 300
	}
	var out bytes.Buffer
	var offsets []int
	obj := func(body string) int {
		offsets = append(offsets, out.Len())
		n := len(offsets)
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", n, body)
		return n
	}
	out.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	// Каталог и дерево страниц — объекты 1 и 2; страницы ссылаются на 2.
	obj("<< /Type /Catalog /Pages 2 0 R >>")
	pagesAt := len(offsets)
	offsets = append(offsets, 0) // место под объект 2
	gs := obj("<< /D << /Type /ExtGState /BM /Darken >> /N << /Type /ExtGState /BM /Normal >> >>")
	var kids []string
	scale := 72 / float64(dpi)
	for _, sh := range sheets {
		content, err := pdfContent(sh, scale)
		if err != nil {
			return err
		}
		stream := obj(fmt.Sprintf("<< /Length %d /Filter /FlateDecode >>\nstream\n%s\nendstream", len(content), content))
		wpt, hpt := float64(sh.Width)*scale, float64(sh.Height)*scale
		page := obj(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.2f %.2f] /Contents %d 0 R /Resources << /ExtGState %d 0 R >> >>",
			wpt, hpt, stream, gs))
		kids = append(kids, fmt.Sprintf("%d 0 R", page))
	}
	offsets[pagesAt] = out.Len()
	fmt.Fprintf(&out, "2 0 obj\n<< /Type /Pages /Kids [%s] /Count %d >>\nendobj\n", joinRefs(kids), len(kids))
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, off := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref)
	_, err := w.Write(out.Bytes())
	return err
}

func joinRefs(refs []string) string {
	var b bytes.Buffer
	for i, r := range refs {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(r)
	}
	return b.String()
}

// pdfContent — сжатый поток рисования страницы. Координаты остаются в пикселях листа:
// матрица переводит их в пункты и переворачивает ось y.
func pdfContent(sh Sheet, scale float64) ([]byte, error) {
	var raw bytes.Buffer
	fmt.Fprintf(&raw, "%.5f 0 0 %.5f 0 %.2f cm\n1 J 1 j\n", scale, -scale, float64(sh.Height)*scale)
	mode := ""
	for _, p := range sh.paths() {
		want := "/N gs\n"
		if p.darken {
			want = "/D gs\n"
		}
		if want != mode {
			raw.WriteString(want)
			mode = want
		}
		fmt.Fprintf(&raw, "%.3f G %s w\n", float64(p.tone)/255, num(p.width))
		for _, part := range p.parts {
			raw.WriteString(num(part[0]) + " " + num(part[1]) + " m")
			for k := 2; k < len(part); k += 2 {
				raw.WriteString(" " + num(part[k]) + " " + num(part[k+1]) + " l")
			}
			raw.WriteString(" S\n")
		}
	}
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	if _, err := zw.Write(raw.Bytes()); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return z.Bytes(), nil
}
