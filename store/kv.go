package store

import (
	"encoding/hex"
	"fmt"
	"iter"
	"os"
	"strings"
)

// KV is a trivial key-value for content addressable store.
// Path format: "<fs>/<bucket>/<key><ext>".
type KV struct {
	fs   FS
}

type Key []byte

func OpenKV(fs FS) KV {
	return KV{fs: fs}
}

func (c *KV) prepareKey(key Key) {
	bucket := fmt.Sprintf("%02x", key[0])
	if _, err := c.fs.Stat(bucket); err != nil {
		c.fs.Mkdir(bucket, 0o700) //nolint:errcheck //ok
	}
}

func (c *KV) LocateKey(key Key, ext string) string {
	return fmt.Sprintf("%02x/%x%s", key[0], key, ext)
}

func (c *KV) StatKey(key Key, ext string) (os.FileInfo, error) {
	return c.fs.Stat(c.LocateKey(key, ext))
}

func (c *KV) WriteKey(key Key, ext string, data []byte, perm os.FileMode) error {
	c.prepareKey(key)
	return c.fs.WriteFile(c.LocateKey(key, ext), data, perm)
}

func (c *KV) ReadKey(key Key, ext string) ([]byte, error) {
	return c.fs.ReadFile(c.LocateKey(key, ext))
}

func (c *KV) MkdirKey(key Key, ext string, perm os.FileMode) error {
	c.prepareKey(key)
	return c.fs.Mkdir(c.LocateKey(key, ext), perm)
}

func (c *KV) SubKey(key Key, ext string) (FS, error) {
	return c.fs.Sub(c.LocateKey(key, ext))
}

func (c *KV) RenameKey(key Key, oldext, newext string) error {
	return c.fs.Rename(c.LocateKey(key, oldext), c.LocateKey(key, newext))
}

func (c *KV) RemoveKey(key Key, ext string) error {
	return c.fs.RemoveAll(c.LocateKey(key, ext))
}

func (c *KV) IterKeys(ext string) iter.Seq2[Key, os.FileInfo] {
	return func(yield func(Key, os.FileInfo) bool) {
		buckets, err := c.fs.ReadDir(".")
		if err != nil {
			return
		}
		for _, bucket := range buckets {
			if !bucket.IsDir() || len(bucket.Name()) != 2 {
				continue
			}
			entries, err := c.fs.ReadDir(bucket.Name())
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if !strings.HasSuffix(entry.Name(), ext) {
					continue
				}
				key, err := hex.DecodeString(strings.TrimSuffix(entry.Name(), ext))
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
