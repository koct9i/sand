package ssh

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"os"

	"github.com/koct9i/sand/ssh"
	"github.com/koct9i/sand/store"
)

func UploadSelf(kv store.KV) (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(path)
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
	return ".cache/sand/" + kv.LocateKey(key, ".sand"), nil
}

func Main(ctx context.Context, host, command string) error {
	remote, err := ssh.NewRemote(ctx, host)
	if err != nil {
		return err
	}

	if err := remote.CreateHomeKV(); err != nil {
		log.Print("error", err)
	}

	kv, err := remote.OpenHomeKV()
	if err != nil {
		return err
	}

	name, err := UploadSelf(kv)
	if err != nil {
		return err
	}
	fmt.Println(name)

	sshSession, err := remote.NewSession()
	if err != nil {
		return err
	}
	defer sshSession.Close()

	var buf bytes.Buffer
	sshSession.Stdout = &buf
	if err := sshSession.Run("~/" + name + " " + command); err != nil {
		log.Fatal("Failed to run: " + err.Error())
	}
	fmt.Println(buf.String())

	return nil
}
