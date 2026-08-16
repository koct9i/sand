package ssh

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"path"

	"github.com/koct9i/sand/ssh"
	"github.com/koct9i/sand/store"
)

func UploadSelf(kv store.KV) (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(self)
	if err != nil {
		return "", err
	}

	h := sha256.New()
	if _, err := h.Write(data); err != nil {
		return "", err
	}

	key := h.Sum(nil)
	info, err := kv.StatKey(key, ".sand")
	if err != nil || info.Size() != int64(len(data)) {
		err = kv.WriteKey(key, ".sand", data, 0o700)
		if err != nil {
			return "", err
		}
	}
	return path.Join(store.RemoteCachePath, kv.LocateKey(key, ".sand")), nil
}

func Main(ctx context.Context, host string, command []string) error {
	remote, err := ssh.NewRemote(ctx, host)
	if err != nil {
		return err
	}

	kv, err := remote.OpenHomeKV(ctx)
	if err != nil {
		return err
	}

	name, err := UploadSelf(kv)
	if err != nil {
		return err
	}
	fmt.Println(name)

	cmd, err := remote.NewCommand("~/"+name, command...)
	if err != nil {
		return err
	}
	cmd.Args[0] = "sand"
	defer cmd.Close()

	var buf bytes.Buffer
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		log.Fatal("Failed to run: " + err.Error())
	}
	fmt.Println(buf.String())

	return nil
}
