package store

import (
	"encoding/hex"
	"fmt"
	"io"
	"iter"
	"os"
	"strings"
)

type Key []byte

// KV is a trivial key-value for content addressable store.
type KV interface {
	LocateKey(key Key, ext string) string
	StatKey(key Key, ext string) (os.FileInfo, error)
	ChmodKey(key Key, ext string, mode os.FileMode) error
	OpenKey(key Key, ext string) (io.ReadCloser, error)
	CreateKey(key Key, ext string) (io.WriteCloser, error)
	ReadKey(key Key, ext string) ([]byte, error)
	WriteKey(key Key, ext string, data []byte, perm os.FileMode) error
	MkdirKey(key Key, ext string, perm os.FileMode) error
	RenameKey(key Key, oldext, newext string) error
	RemoveKey(key Key, ext string) error
	IterKeys(ext string) iter.Seq2[Key, os.FileInfo]
	OpenDirKey(key Key, ext string) (Root, error)
}

type KVOptions struct {
	Create bool
}

// rootKV implements KV in fs root: "<root>/<key0>/<key><ext>".
type rootKV struct {
	root Root
}

func OpenRootKV(root Root, name string, opts KVOptions) (*rootKV, error) {
	if name != "" {
		kvRoot, err := root.OpenDir(name)
		if err != nil && os.IsNotExist(err) && opts.Create {
			if err = root.Mkdir(name, 0o700); err == nil {
				kvRoot, err = root.OpenDir(name)
			}
		}
		if err != nil {
			return nil, err
		}
		root = kvRoot
	}
	return &rootKV{root: root}, nil
}

func (c *rootKV) prepareKey(key Key) {
	bucket := fmt.Sprintf("%02x", key[0])
	if _, err := c.root.Stat(bucket); err != nil {
		c.root.Mkdir(bucket, 0o700) //nolint:errcheck //ok
	}
}

func (c *rootKV) LocateKey(key Key, ext string) string {
	return fmt.Sprintf("%02x/%x%s", key[0], key[1:], ext)
}

func (c *rootKV) StatKey(key Key, ext string) (os.FileInfo, error) {
	return c.root.Stat(c.LocateKey(key, ext))
}

func (c *rootKV) ChmodKey(key Key, ext string, mode os.FileMode) error {
	return c.root.Chmod(c.LocateKey(key, ext), mode)
}

func (c *rootKV) OpenKey(key Key, ext string) (io.ReadCloser, error) {
	return c.root.Open(c.LocateKey(key, ext))
}

func (c *rootKV) CreateKey(key Key, ext string) (io.WriteCloser, error) {
	return c.root.OpenFile(c.LocateKey(key, ext), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0x600)
}

func (c *rootKV) ReadKey(key Key, ext string) ([]byte, error) {
	return c.root.ReadFile(c.LocateKey(key, ext))
}

func (c *rootKV) WriteKey(key Key, ext string, data []byte, perm os.FileMode) error {
	c.prepareKey(key)
	return c.root.WriteFile(c.LocateKey(key, ext), data, perm)
}

func (c *rootKV) MkdirKey(key Key, ext string, perm os.FileMode) error {
	c.prepareKey(key)
	return c.root.Mkdir(c.LocateKey(key, ext), perm)
}

func (c *rootKV) OpenDirKey(key Key, ext string) (Root, error) {
	return c.root.OpenDir(c.LocateKey(key, ext))
}

func (c *rootKV) RenameKey(key Key, oldext, newext string) error {
	return c.root.Rename(c.LocateKey(key, oldext), c.LocateKey(key, newext))
}

func (c *rootKV) RemoveKey(key Key, ext string) error {
	return c.root.RemoveAll(c.LocateKey(key, ext))
}

func (c *rootKV) IterKeys(ext string) iter.Seq2[Key, os.FileInfo] {
	return func(yield func(Key, os.FileInfo) bool) {
		buckets, err := c.root.ReadDir(".")
		if err != nil {
			return
		}
		for _, bucket := range buckets {
			if !bucket.IsDir() || len(bucket.Name()) != 2 {
				continue
			}
			entries, err := c.root.ReadDir(bucket.Name())
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if !strings.HasSuffix(entry.Name(), ext) {
					continue
				}
				key, err := hex.DecodeString(bucket.Name()+strings.TrimSuffix(entry.Name(), ext))
				if err != nil {
					continue
				}
				info, err := entry.Info()
				if err != nil {
					continue
				}
				if !yield(key, info) {
					return
				}
			}
		}
	}
}
