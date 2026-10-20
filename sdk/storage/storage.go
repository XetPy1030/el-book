// Package storage хранит данные приложений на пользовательском разделе книги.
// Запись всегда атомарная: временный файл, fsync, переименование поверх старого;
// предыдущая версия остаётся рядом как .bak и подхватывается, если основной файл испорчен.
// Приложение не видит путей: оно получает App со своими операциями.
package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Store — корень данных платформы.
type Store struct {
	root string
}

// Open открывает (и при необходимости создаёт) корень данных.
func Open(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("создать корень данных %s: %w", root, err)
	}
	return &Store{root: root}, nil
}

// Root — путь к корню; нужен только оболочке и журналу.
func (s *Store) Root() string { return s.root }

// App возвращает хранилище приложения id.
func (s *Store) App(id string) *App {
	return &App{id: id, dir: filepath.Join(s.root, "apps", id), export: filepath.Join(s.root, "export", id)}
}

// App — хранилище одного приложения: настройки, документы, кэш, выгрузка.
type App struct {
	id     string
	dir    string
	export string
}

// ID — идентификатор приложения.
func (a *App) ID() string { return a.id }

type envelope struct {
	Version int             `json:"version"`
	Data    json.RawMessage `json:"data"`
}

// ErrNoSettings — настроек ещё нет: приложение запущено впервые.
var ErrNoSettings = errors.New("настроек ещё нет")

// LoadSettings читает настройки в v и возвращает версию схемы, с которой они записаны.
// Если основной файл испорчен, читается предыдущая версия.
func (a *App) LoadSettings(v any) (int, error) {
	path := filepath.Join(a.dir, "settings.json")
	for _, p := range []string{path, path + ".bak"} {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var env envelope
		if json.Unmarshal(data, &env) != nil || env.Data == nil {
			continue
		}
		if err := json.Unmarshal(env.Data, v); err != nil {
			continue
		}
		return env.Version, nil
	}
	return 0, ErrNoSettings
}

// SaveSettings записывает настройки с версией схемы version.
func (a *App) SaveSettings(version int, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("настройки %s: %w", a.id, err)
	}
	out, err := json.MarshalIndent(envelope{Version: version, Data: data}, "", "  ")
	if err != nil {
		return fmt.Errorf("настройки %s: %w", a.id, err)
	}
	return WriteAtomic(filepath.Join(a.dir, "settings.json"), out, true)
}

// clean проверяет, что имя документа не выходит из папки приложения.
func clean(name string) (string, error) {
	c := filepath.Clean(filepath.FromSlash(name))
	if c == "." || filepath.IsAbs(c) || c == ".." || strings.HasPrefix(c, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("недопустимое имя %q", name)
	}
	return c, nil
}

func (a *App) dataPath(name string) (string, error) {
	c, err := clean(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(a.dir, "data", c), nil
}

// WriteFile атомарно записывает документ name (путь внутри папки документов, через «/»).
func (a *App) WriteFile(name string, data []byte) error {
	p, err := a.dataPath(name)
	if err != nil {
		return err
	}
	return WriteAtomic(p, data, false)
}

// SetModTime ставит документу время изменения: так список документов упорядочен по времени
// платформы, а не по часам файловой системы.
func (a *App) SetModTime(name string, t time.Time) error {
	p, err := a.dataPath(name)
	if err != nil {
		return err
	}
	return os.Chtimes(p, t, t)
}

// ReadFile читает документ name.
func (a *App) ReadFile(name string) ([]byte, error) {
	p, err := a.dataPath(name)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(p)
}

// Remove удаляет документ или папку документов name.
func (a *App) Remove(name string) error {
	p, err := a.dataPath(name)
	if err != nil {
		return err
	}
	return os.RemoveAll(p)
}

// Entry — документ в папке.
type Entry struct {
	Name    string
	Size    int64
	ModTime int64 // Unix-время, секунды
	Dir     bool
}

// List возвращает документы в папке dir (пусто — корень документов), новые первыми.
// Служебные временные и резервные файлы не показываются.
func (a *App) List(dir string) ([]Entry, error) {
	p := filepath.Join(a.dir, "data")
	if dir != "" {
		var err error
		if p, err = a.dataPath(dir); err != nil {
			return nil, err
		}
	}
	items, err := os.ReadDir(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, it := range items {
		n := it.Name()
		if strings.HasPrefix(n, ".") || strings.HasSuffix(n, ".bak") {
			continue
		}
		info, err := it.Info()
		if err != nil {
			continue
		}
		out = append(out, Entry{Name: n, Size: info.Size(), ModTime: info.ModTime().Unix(), Dir: it.IsDir()})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ModTime != out[j].ModTime {
			return out[i].ModTime > out[j].ModTime
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// Export записывает файл в папку выгрузки, которую видно по USB, и возвращает путь от корня данных.
func (a *App) Export(name string, data []byte) (string, error) {
	c, err := clean(name)
	if err != nil {
		return "", err
	}
	if err := WriteAtomic(filepath.Join(a.export, c), data, false); err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Join("export", a.id, c)), nil
}

// CacheDir — папка кэша, которую можно удалить в любой момент.
func (a *App) CacheDir() (string, error) {
	p := filepath.Join(a.dir, "cache")
	return p, os.MkdirAll(p, 0o755)
}

// WriteAtomic записывает файл так, чтобы при отключении питания остался либо старый,
// либо новый файл целиком. keepBackup сохраняет прежнюю версию как .bak.
func WriteAtomic(path string, data []byte, keepBackup bool) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("создать папку %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".tmp-"+filepath.Base(path)+"-*")
	if err != nil {
		return fmt.Errorf("временный файл для %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // после успешного переименования файла уже нет
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("записать %s: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("сбросить на диск %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("закрыть %s: %w", path, err)
	}
	if keepBackup {
		if _, err := os.Stat(path); err == nil {
			if err := os.Rename(path, path+".bak"); err != nil {
				return fmt.Errorf("резервная копия %s: %w", path, err)
			}
		}
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("заменить %s: %w", path, err)
	}
	syncDir(dir)
	return nil
}

// syncDir сбрасывает запись о переименовании. На FAT это может не поддерживаться — ошибка не важна.
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		d.Close()
	}
}
