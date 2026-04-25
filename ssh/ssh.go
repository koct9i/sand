package ssh

import (
	"context"
	"io/fs"
	"net"
	"os"

	gossh "golang.org/x/crypto/ssh"
	gosshagent "golang.org/x/crypto/ssh/agent"

	"github.com/pkg/sftp/v2"

	"github.com/koct9i/sand/store"
)

type Remote struct {
	sshClient  *gossh.Client
	sftpClient *sftp.Client
}

func NewRemote(ctx context.Context, host string) (*Remote, error) {
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

	addr := host + ":22"
	sshDialer := net.Dialer{}
	conn, err := sshDialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}

	sshConn, chans, reqs, err := gossh.NewClientConn(conn, addr, config)
	if err != nil {
		return nil, err
	}
	sshClient := gossh.NewClient(sshConn, chans, reqs)

	sftpClient, err := sftp.NewClient(ctx, sshClient)
	if err != nil {
		sshConn.Close()
		return nil, err
	}

	return &Remote{
		sshClient:  sshClient,
		sftpClient: sftpClient,
	}, nil
}

type RemoteFS struct {
	client *sftp.Client
	prefix string
}

func (r *RemoteFS) Stat(name string) (os.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}
	return r.client.Stat(r.prefix + name)
}

func (r *RemoteFS) ReadFile(name string) ([]byte, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrInvalid}
	}
	return r.client.ReadFile(r.prefix + name)
}

func (r *RemoteFS) WriteFile(name string, data []byte, perm fs.FileMode) error {
	if !fs.ValidPath(name) {
		return &fs.PathError{Op: "write", Path: name, Err: fs.ErrInvalid}
	}
	return r.client.WriteFile(r.prefix+name, data, perm)
}

func (r *RemoteFS) ReadDir(name string) ([]os.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	return r.client.ReadDir(r.prefix + name)
}

func (r *RemoteFS) Sub(name string) (store.FS, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "sub", Path: name, Err: fs.ErrInvalid}
	}
	return &RemoteFS{client: r.client, prefix: r.prefix + name}, nil
}

func (r *RemoteFS) Mkdir(name string, perm fs.FileMode) error {
	if !fs.ValidPath(name) {
		return &fs.PathError{Op: "mkdir", Path: name, Err: fs.ErrInvalid}
	}
	return r.client.Mkdir(r.prefix+name, perm)
}

func (r *RemoteFS) Rename(oldname, newname string) error {
	if !fs.ValidPath(oldname) {
		return &fs.PathError{Op: "rename", Path: oldname, Err: fs.ErrInvalid}
	}
	if !fs.ValidPath(newname) {
		return &fs.PathError{Op: "rename", Path: newname, Err: fs.ErrInvalid}
	}
	return r.client.Rename(r.prefix+oldname, r.prefix+newname)
}

func (r *RemoteFS) RemoveAll(name string) error {
	if !fs.ValidPath(name) {
		return &fs.PathError{Op: "remove", Path: name, Err: fs.ErrInvalid}
	}
	return r.client.Remove(r.prefix + name)
}

func (r *Remote) OpenFS(root string) (store.FS, error) {
	if root != "" && root[len(root)-1] != '/' {
		root += "/"
	}
	return &RemoteFS{client: r.sftpClient, prefix: root}, nil
}

func (r *Remote) OpenKV(root string) (store.KV, error) {
	if err := r.sftpClient.MkdirAll(root, 0o700); err != nil {
		return store.KV{}, err
	}
	fs, err := r.OpenFS(root)
	if err != nil {
		return store.KV{}, nil
	}
	return store.OpenKV(fs), nil
}

func (r *Remote) OpenHomeKV() (store.KV, error) {
	return r.OpenKV(".cache/sand")
}

func (r *Remote) NewSession() (*gossh.Session, error) {
	return r.sshClient.NewSession()
}
