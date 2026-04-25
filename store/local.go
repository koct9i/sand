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

type FS interface {
	Stat(name string) (os.FileInfo, error)
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm fs.FileMode) error
	ReadDir(name string) ([]os.DirEntry, error)
	Sub(name string) (FS, error)
	Mkdir(name string, perm fs.FileMode) error
	Rename(oldname, newname string) error
	RemoveAll(name string) error
}

type LocalFS struct {
	*os.Root
}

func (f LocalFS) ReadDir(name string) ([]os.DirEntry, error) {
	return fs.ReadDir(f.FS(), name)
}

func (f LocalFS) Sub(name string) (FS, error) {
	root, err := f.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	return LocalFS{root}, nil
}

func OpenLocalFS(path string) (FS, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	return LocalFS{Root: root}, nil
}

func OpenLocalKV(path string) (KV, error) {
	err := os.MkdirAll(path, 0o700)
	if err != nil {
		return KV{}, err
	}
	store, err := OpenLocalFS(path)
	if err != nil {
		return KV{}, err
	}
	return OpenKV(store), nil
}

func OpenHomeKV() (KV, error) {
	return OpenLocalKV(filepath.Join(xdg.CacheHome, LocalName))
}
