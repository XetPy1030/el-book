// Package wiki — Википедия: поиск, случайная статья, чтение страницами. Открытые статьи
// сохраняются на книге и читаются без сети; список — недавние первыми.
package wiki

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"el-book/sdk/app"
	"el-book/sdk/icons"
	"el-book/sdk/kit"
	"el-book/sdk/ru"
	"el-book/sdk/ui"
	"el-book/sdk/web"
)

const api = "https://ru.wikipedia.org/w/api.php"

// errNoArticle — статьи с таким названием нет.
var errNoArticle = errors.New("такой статьи нет")

// App — приложение «Википедия».
type App struct {
	main *kit.ListScreen
}

// New создаёт приложение.
func New() *App { return &App{} }

func (a *App) Manifest() app.Manifest {
	return app.Manifest{ID: "wiki", Title: "Википедия", Icon: icons.Globe, Landscape: true}
}

// article — сохранённая статья.
type article struct {
	Title  string `json:"title"`
	Text   string `json:"text"` // простой текст с заголовками «== Раздел ==»
	Opened int64  `json:"opened"`
}

func articleFile(title string) string {
	sum := sha1.Sum([]byte(strings.ToLower(title)))
	return "articles/" + hex.EncodeToString(sum[:10]) + ".json"
}

func (a *App) Start(ctx *app.Context) app.Screen {
	a.main = &kit.ListScreen{
		Title: "Википедия",
		Root:  true,
		Empty: "Найдите статью кнопкой поиска или откройте случайную. Открытые статьи сохраняются здесь и читаются без сети.",
		Actions: []*ui.Button{
			{Icon: icons.Dice, Style: ui.Flat, OnTap: func() { a.random(ctx) }},
			{Icon: icons.Search, Style: ui.Flat, OnTap: func() { a.search(ctx) }},
		},
		Items: func(ctx *app.Context) []ui.ListItem { return a.saved(ctx) },
	}
	return a.main
}

// saved — сохранённые статьи, недавно открытые первыми.
func (a *App) saved(ctx *app.Context) []ui.ListItem {
	st := ctx.Storage()
	files, err := st.List("articles")
	if err != nil {
		ctx.Log().Error("статьи", "err", err)
	}
	var arts []article
	for _, f := range files {
		data, err := st.ReadFile("articles/" + f.Name)
		if err != nil {
			continue
		}
		var art article
		if json.Unmarshal(data, &art) == nil && art.Title != "" {
			arts = append(arts, art)
		}
	}
	sort.Slice(arts, func(i, j int) bool { return arts[i].Opened > arts[j].Opened })
	now := ctx.Now()
	var items []ui.ListItem
	for _, art := range arts {
		art := art
		items = append(items, ui.ListItem{
			Title: art.Title, Subtitle: "открыта " + ru.Ago(time.Unix(art.Opened, 0), now), Icon: icons.Book,
			OnTap: func() { a.open(ctx, art.Title) },
			OnLongPress: func() {
				ctx.Confirm("Удалить «"+art.Title+"»?", "Статья удалится с книги; в Википедии она останется.", "Удалить", func() {
					_ = st.Remove(articleFile(art.Title))
					a.main.Refresh(ctx)
				})
			},
		})
	}
	return items
}

// searchResult — ответ поиска.
type searchResult struct {
	Query struct {
		Search []struct {
			Title   string `json:"title"`
			Snippet string `json:"snippet"`
		} `json:"search"`
	} `json:"query"`
}

func (a *App) search(ctx *app.Context) {
	kit.Prompt(ctx, kit.PromptOptions{Title: "Поиск в Википедии", Placeholder: "Например: Kobo", OK: "Найти"}, func(q string) {
		screen := &kit.ListScreen{Title: q, Subtitle: "ищем…", Empty: "Ищем…"}
		var results []ui.ListItem
		screen.Items = func(*app.Context) []ui.ListItem { return results }
		ctx.Push(screen)
		kit.Load(ctx, func(bg context.Context) (searchResult, error) {
			var r searchResult
			err := ctx.Web().GetJSON(bg, web.URL(api, "action", "query", "list", "search", "srsearch", q,
				"srlimit", "30", "format", "json", "formatversion", "2", "utf8", "1"), &r)
			return r, err
		}, func(r searchResult, err error) {
			if err != nil {
				screen.Empty = web.Explain(err)
				screen.Bar().SetTitle(ctx, q, "")
				screen.Refresh(ctx)
				return
			}
			screen.Empty = "Ничего не нашлось. Попробуйте другие слова."
			for _, s := range r.Query.Search {
				title := s.Title
				results = append(results, ui.ListItem{Title: title, Subtitle: web.StripTags(s.Snippet), OnTap: func() { a.open(ctx, title) }})
			}
			screen.Bar().SetTitle(ctx, q, ru.Count(len(results), "статья", "статьи", "статей"))
			screen.Refresh(ctx)
		})
	})
}

// randomResult — ответ «случайная статья».
type randomResult struct {
	Query struct {
		Random []struct {
			Title string `json:"title"`
		} `json:"random"`
	} `json:"query"`
}

func (a *App) random(ctx *app.Context) {
	ctx.Toast("Выбираем статью…")
	kit.Load(ctx, func(bg context.Context) (string, error) {
		var r randomResult
		if err := ctx.Web().GetJSON(bg, web.URL(api, "action", "query", "list", "random", "rnnamespace", "0",
			"rnlimit", "1", "format", "json", "formatversion", "2"), &r); err != nil {
			return "", err
		}
		if len(r.Query.Random) == 0 {
			return "", errors.New("Википедия не дала статью")
		}
		return r.Query.Random[0].Title, nil
	}, func(title string, err error) {
		if err != nil {
			ctx.Fail(web.Explain(err), err)
			return
		}
		a.open(ctx, title)
	})
}

// extractResult — текст статьи.
type extractResult struct {
	Query struct {
		Pages []struct {
			Title   string `json:"title"`
			Extract string `json:"extract"`
			Missing bool   `json:"missing"`
		} `json:"pages"`
	} `json:"query"`
}

// open показывает статью: сохранённую — сразу, затем обновляет из сети.
func (a *App) open(ctx *app.Context, title string) {
	st := ctx.Storage()
	screen := &kit.ReaderScreen{Title: title, Subtitle: "загружаем…", Blocks: []ui.Block{{Kind: ui.Heading, Text: title}}}
	var saved article
	if data, err := st.ReadFile(articleFile(title)); err == nil && json.Unmarshal(data, &saved) == nil {
		screen.Blocks = blocks(saved.Title, saved.Text)
		screen.Subtitle = "сохранена на книге"
	}
	ctx.Push(screen)
	if saved.Text != "" && ctx.Now().Sub(time.Unix(saved.Opened, 0)) < 7*24*time.Hour {
		a.store(ctx, saved) // обновить время открытия; сеть не нужна
		return
	}
	kit.Load(ctx, func(bg context.Context) (article, error) {
		var r extractResult
		err := ctx.Web().GetJSON(bg, web.URL(api, "action", "query", "prop", "extracts", "explaintext", "1",
			"exsectionformat", "wiki", "redirects", "1", "titles", title, "format", "json", "formatversion", "2"), &r)
		if err != nil {
			return article{}, err
		}
		if len(r.Query.Pages) == 0 || r.Query.Pages[0].Missing || r.Query.Pages[0].Extract == "" {
			return article{}, errNoArticle
		}
		p := r.Query.Pages[0]
		return article{Title: p.Title, Text: p.Extract}, nil
	}, func(art article, err error) {
		bar := screen.Bar()
		if err != nil {
			if saved.Text != "" {
				bar.SetTitle(ctx, saved.Title, "сохранена на книге · не обновилась")
				return
			}
			msg := web.Explain(err)
			if errors.Is(err, errNoArticle) {
				msg = "Статьи «" + title + "» в Википедии нет."
			}
			screen.SetBlocks(ctx, []ui.Block{{Kind: ui.Heading, Text: title}, {Kind: ui.Para, Text: msg}})
			bar.SetTitle(ctx, title, "")
			return
		}
		a.store(ctx, art)
		bar.SetTitle(ctx, art.Title, "сохранена на книге")
		screen.SetBlocks(ctx, blocks(art.Title, art.Text))
	})
}

func (a *App) store(ctx *app.Context, art article) {
	art.Opened = ctx.Now().Unix()
	data, _ := json.Marshal(art)
	if err := ctx.Storage().WriteFile(articleFile(art.Title), data); err != nil {
		ctx.Log().Warn("статья не сохранилась", "err", err)
	}
	a.main.Refresh(ctx)
}

// blocks превращает текст статьи в абзацы: «== Раздел ==» — заголовок, «=== Подраздел ===»
// — подзаголовок. Пустые разделы в конце («Ссылки», «Литература») пропускаются.
func blocks(title, text string) []ui.Block {
	out := []ui.Block{{Kind: ui.Heading, Text: title}}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "==") && strings.HasSuffix(line, "==") {
			level := len(line) - len(strings.TrimLeft(line, "="))
			name := strings.TrimSpace(strings.Trim(line, "="))
			if name == "" {
				continue
			}
			kind := ui.Subheading
			if level <= 2 {
				kind = ui.Heading
			}
			out = append(out, ui.Block{Kind: kind, Text: name})
			continue
		}
		out = append(out, ui.Block{Kind: ui.Para, Text: line})
	}
	// Заголовки без текста под ними — разделы со ссылками, которых в простом тексте нет.
	for len(out) > 1 && out[len(out)-1].Kind != ui.Para {
		out = out[:len(out)-1]
	}
	cleaned := out[:0]
	for i, b := range out {
		if b.Kind != ui.Para && i > 0 && i+1 < len(out) && out[i+1].Kind != ui.Para && levelOf(out[i+1]) <= levelOf(b) {
			continue
		}
		cleaned = append(cleaned, b)
	}
	return cleaned
}

func levelOf(b ui.Block) int {
	if b.Kind == ui.Heading {
		return 1
	}
	return 2
}
