package ssh

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/go-logr/logr"

	gossh "golang.org/x/crypto/ssh"
	gosshagent "golang.org/x/crypto/ssh/agent"

	gosftp "github.com/pkg/sftp/v2"

	"github.com/koct9i/sand/store"
)

type Remote struct {
	ssh *gossh.Client
	logger logr.Logger
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
		logger: logr.FromContextOrDiscard(ctx).WithValues("address", address),
	}, nil
}

func (r *Remote) Name() string {
	return fmt.Sprintf("ssh://%s@%s", r.ssh.User(), r.ssh.RemoteAddr().String())
}

func (r *Remote) Close() error {
	return r.ssh.Close()
}

func (r *Remote) OpenDir(ctx context.Context, name string) (store.Root, error) {
	sftp, err := gosftp.NewClient(ctx, r.ssh)
	if err != nil {
		return nil, err
	}
	prefix := name
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	if stat, err := sftp.Stat(prefix + "."); err != nil {
		return nil, fmt.Errorf("kv root %v: %w", name, err)
	} else if !stat.IsDir() {
		return nil, fmt.Errorf("kv root %v is not a directory", name)
	}
	r.logger.Info("Open remote dir", "prefix", prefix)
	return &remoteRoot{
		ssh: r.ssh,
		sftp: sftp,
		logger: r.logger.WithValues("prefix", prefix),
		prefix: prefix,
	}, nil
}

func (r *Remote) OpenKV(ctx context.Context, name string, opts store.KVOptions) (store.KV, error) {
	root, err := r.OpenDir(ctx, "")
	if err != nil {
		return nil, err
	}
	return store.OpenRootKV(root, name, opts)
}

func (r *Remote) OpenHomeKV(ctx context.Context) (store.KV, error) {
	root, err := r.OpenDir(ctx, "")
	if err != nil {
		return nil, err
	}
	return store.OpenRootKV(root, store.RemoteCachePath, store.KVOptions{Create: true})
}

func (r *Remote) NewCommand(name string, args ...string) (*Command, error) {
	s, err := r.ssh.NewSession()
	if err != nil {
		return nil, err
	}
	return &Command{
		Session: s,
		Logger: r.logger,
		Path: name,
		Args: append([]string{name}, args...),
	}, nil
}
