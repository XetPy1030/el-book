package storage

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Кэш приложения — файлы, которые можно потерять: ответы сети, миниатюры. Запись атомарная,
// как у документов, но без резервной копии.

// WriteCache записывает файл кэша name.
func (a *App) WriteCache(name string, data []byte) error {
	c, err := clean(name)
	if err != nil {
		return err
	}
	return WriteAtomic(filepath.Join(a.dir, "cache", c), data, false)
}

// ReadCache читает файл кэша и время его записи. Отсутствие файла — ошибка fs.ErrNotExist.
func (a *App) ReadCache(name string) ([]byte, time.Time, error) {
	c, err := clean(name)
	if err != nil {
		return nil, time.Time{}, err
	}
	p := filepath.Join(a.dir, "cache", c)
	info, err := os.Stat(p)
	if err != nil {
		return nil, time.Time{}, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, time.Time{}, err
	}
	return data, info.ModTime(), nil
}

// ClearCache удаляет весь кэш приложения.
func (a *App) ClearCache() error {
	err := os.RemoveAll(filepath.Join(a.dir, "cache"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// Usage — сколько байт занимают данные приложения: документы, настройки и кэш.
func (a *App) Usage() int64 {
	var total int64
	_ = filepath.WalkDir(a.dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

// Exists — есть ли документ name.
func (a *App) Exists(name string) bool {
	p, err := a.dataPath(name)
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}
