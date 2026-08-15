package ssh

import (
	"fmt"
	"io"
	"io/fs"
	"os"

	gossh "golang.org/x/crypto/ssh"

	gosftp "github.com/pkg/sftp/v2"

	"github.com/koct9i/sand/store"
)

type remoteRoot struct {
	ssh    *gossh.Client
	sftp   *gosftp.Client
	prefix string
}

var (
	_ store.Root = (*remoteRoot)(nil)
)

func (r *remoteRoot) Name() string {
	if r.prefix[0] == '/' {
		return fmt.Sprintf("sftp://%s%s", r.ssh.RemoteAddr().String(), r.prefix)
	}
	return fmt.Sprintf("sftp://%s/~%s/%s", r.ssh.RemoteAddr().String(), r.ssh.User(), r.prefix)
}

func (r *remoteRoot) Close() error {
	return r.sftp.Close()
}

func (r *remoteRoot) Stat(name string) (os.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}
	return r.sftp.Stat(r.prefix + name)
}

func (r *remoteRoot) Chmod(name string, mode fs.FileMode) error {
	if !fs.ValidPath(name) {
		return &fs.PathError{Op: "chmod", Path: name, Err: fs.ErrInvalid}
	}
	return r.sftp.Chmod(r.prefix+name, mode)
}

func (r *remoteRoot) Open(name string) (io.ReadCloser, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	return r.sftp.Open(name)
}

func (r *remoteRoot) OpenFile(name string, flag int, mode fs.FileMode) (io.ReadWriteCloser, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "openfile", Path: name, Err: fs.ErrInvalid}
	}
	return r.sftp.OpenFile(name, flag, mode)
}

func (r *remoteRoot) ReadFile(name string) ([]byte, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrInvalid}
	}
	return r.sftp.ReadFile(r.prefix + name)
}

func (r *remoteRoot) WriteFile(name string, data []byte, perm fs.FileMode) error {
	if !fs.ValidPath(name) {
		return &fs.PathError{Op: "write", Path: name, Err: fs.ErrInvalid}
	}
	return r.sftp.WriteFile(r.prefix+name, data, perm)
}

func (r *remoteRoot) ReadDir(name string) ([]os.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	return r.sftp.ReadDir(r.prefix + name)
}

func (r *remoteRoot) OpenDir(name string) (store.Root, error) {
	if stat, err := r.Stat(name); err != nil {
		return nil, err
	} else if !stat.IsDir() {
		return nil, &fs.PathError{Op: "opendir", Path: name, Err: fs.ErrInvalid}
	}
	return &remoteRoot{ssh: r.ssh, sftp: r.sftp, prefix: r.prefix + name}, nil
}

func (r *remoteRoot) Mkdir(name string, perm fs.FileMode) error {
	if !fs.ValidPath(name) {
		return &fs.PathError{Op: "mkdir", Path: name, Err: fs.ErrInvalid}
	}
	return r.sftp.Mkdir(r.prefix+name, perm)
}

func (r *remoteRoot) Rename(oldname, newname string) error {
	if !fs.ValidPath(oldname) {
		return &fs.PathError{Op: "rename", Path: oldname, Err: fs.ErrInvalid}
	}
	if !fs.ValidPath(newname) {
		return &fs.PathError{Op: "rename", Path: newname, Err: fs.ErrInvalid}
	}
	return r.sftp.Rename(r.prefix+oldname, r.prefix+newname)
}

func (r *remoteRoot) RemoveAll(name string) error {
	if !fs.ValidPath(name) {
		return &fs.PathError{Op: "remove", Path: name, Err: fs.ErrInvalid}
	}
	return r.sftp.Remove(r.prefix + name)
}
