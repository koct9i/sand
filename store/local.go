package store

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

var (
	LocalName = "sand"
)

type LocalStoreFS struct {
	*os.Root
}

func (f LocalStoreFS) ReadDir(name string) ([]os.DirEntry, error) {
	return fs.ReadDir(f.FS(), name)
}

func OpenLocalKV(path string) (KV, error) {
	err := os.MkdirAll(path, 0o700)
	if err != nil {
		return KV{}, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return KV{}, err
	}
	return OpenKV(LocalStoreFS{Root: root}, ""), nil
}

func OpenHomeKV() (KV, error) {
	return OpenLocalKV(filepath.Join(xdg.CacheHome, LocalName))
}
