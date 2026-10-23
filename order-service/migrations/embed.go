// Package migrations встраивает файлы миграций в бинарник, чтобы сервис
// знал, какая версия схемы ему нужна.
package migrations

import (
	"embed"
	"errors"
	"io/fs"
	"strconv"
	"strings"
)

//go:embed goose/*.sql
var FS embed.FS

// Latest — номер последней миграции в этом образе: схема ниже этой версии
// сервису не подходит.
func Latest() (int64, error) {
	entries, err := fs.ReadDir(FS, "goose")
	if err != nil {
		return 0, err
	}
	var latest int64
	for _, e := range entries {
		name := e.Name()
		i := strings.IndexByte(name, '_')
		if i <= 0 {
			continue
		}
		if v, err := strconv.ParseInt(name[:i], 10, 64); err == nil && v > latest {
			latest = v
		}
	}
	if latest == 0 {
		return 0, errors.New("no migrations embedded")
	}
	return latest, nil
}
