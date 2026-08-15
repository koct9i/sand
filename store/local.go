package store

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

var (
	LocalCacheName  = "sand"
	RemoteCachePath = ".cache/sand"
)

type Root interface {
	Name() string
	Close() error

	Stat(name string) (fs.FileInfo, error)
	Chmod(name string, mode fs.FileMode) error
	Open(name string) (io.ReadCloser, error)
	OpenFile(name string, flag int, perm fs.FileMode) (io.ReadWriteCloser, error)
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm fs.FileMode) error
	OpenDir(name string) (Root, error)
	ReadDir(name string) ([]os.DirEntry, error)
	Mkdir(name string, perm fs.FileMode) error
	Rename(oldname, newname string) error
	RemoveAll(name string) error
}

type localRoot struct {
	*os.Root
}

func (f localRoot) Open(name string) (io.ReadCloser, error) {
	return f.Root.Open(name)
}

func (f localRoot) OpenFile(name string, flag int, perm fs.FileMode) (io.ReadWriteCloser, error) {
	return f.Root.OpenFile(name, flag, perm)
}

func (f localRoot) ReadDir(name string) ([]os.DirEntry, error) {
	return fs.ReadDir(f.FS(), name)
}

func (f localRoot) OpenDir(name string) (Root, error) {
	root, err := f.Root.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	return localRoot{root}, nil
}

func OpenLocalRoot(name string) (Root, error) {
	root, err := os.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	return localRoot{Root: root}, nil
}

func OpenLocalKV(name string, opts KVOptions) (KV, error) {
	if opts.Create {
		err := os.MkdirAll(name, 0o700)
		if err != nil {
			return nil, err
		}
	}
	root, err := OpenLocalRoot(name)
	if err != nil {
		return nil, err
	}
	return OpenRootKV(root, "", opts)
}

func OpenHomeKV() (KV, error) {
	return OpenLocalKV(filepath.Join(xdg.CacheHome, LocalCacheName), KVOptions{Create: true})
}

func OpenRuntimeRoot() (Root, error) {
	runtimeRoot := filepath.Join(xdg.RuntimeDir, LocalCacheName)
	if err := os.MkdirAll(runtimeRoot, 0o700); err != nil {
		return nil, err
	}
	return OpenLocalRoot(runtimeRoot)
}
