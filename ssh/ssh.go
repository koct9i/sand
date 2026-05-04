package ssh

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"

	gossh "golang.org/x/crypto/ssh"
	gosshagent "golang.org/x/crypto/ssh/agent"

	gosftp "github.com/pkg/sftp/v2"

	"github.com/koct9i/sand/store"
)

type Remote struct {
	ssh *gossh.Client
}

type remoteRoot struct {
	ssh    *gossh.Client
	sftp   *gosftp.Client
	prefix string
}

func NewRemote(ctx context.Context, address string) (*Remote, error) {
	var agentDialer net.Dialer
	agentConn, err := agentDialer.DialContext(ctx, "unix", os.Getenv("SSH_AUTH_SOCK"))
	if err != nil {
		return nil, err
	}
	agentClient := gosshagent.NewClient(agentConn)

	config := &gossh.ClientConfig{
		User: os.Getenv("USER"),
		Auth: []gossh.AuthMethod{
			gossh.PublicKeysCallback(agentClient.Signers),
		},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(), // FIXME
	}

	if _, _, err := net.SplitHostPort(address); err != nil {
		address = net.JoinHostPort(address, "22")
	}

	sshDialer := net.Dialer{}
	conn, err := sshDialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}

	sshConn, chans, reqs, err := gossh.NewClientConn(conn, address, config)
	if err != nil {
		return nil, err
	}
	ssh := gossh.NewClient(sshConn, chans, reqs)

	return &Remote{
		ssh: ssh,
	}, nil
}

func (r *Remote) Name() string {
	return fmt.Sprintf("ssh://%s@%s", r.ssh.User(), r.ssh.RemoteAddr().String())
}

func (r *Remote) Close() error {
	return r.ssh.Close()
}

func (r *Remote) OpenRoot(name string) (store.Root, error) {
	if name != "" && name[len(name)-1] != '/' {
		name += "/"
	}
	sftp, err := gosftp.NewClient(context.Background(), r.ssh)
	if err != nil {
		return nil, err
	}
	return &remoteRoot{ssh: r.ssh, sftp: sftp, prefix: name}, nil
}

func (r *Remote) OpenKV(name string) (store.KV, error) {
	root, err := r.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	return store.OpenRootKV(root)
}

func (r *Remote) CreateHomeKV() error {
	root, err := r.OpenRoot("")
	if err != nil {
		return err
	}
	defer root.Close()
	return store.CreateRootKV(root, ".cache/sand")
}

func (r *Remote) OpenHomeKV() (store.KV, error) {
	return r.OpenKV(".cache/sand")
}

func (r *Remote) NewSession() (*gossh.Session, error) {
	return r.ssh.NewSession()
}

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

func (r *remoteRoot) OpenRoot(name string) (store.Root, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "sub", Path: name, Err: fs.ErrInvalid}
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
