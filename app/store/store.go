package store

import (
	"context"
	"fmt"

	"github.com/koct9i/sand/ssh"
	"github.com/koct9i/sand/store"
)

func Main(ctx context.Context) error {
	remote, err := ssh.NewRemote(ctx, "localhost")
	if err != nil {
		return err
	}
	kv, err := remote.OpenHomeKV()
	if err != nil {
		return err
	}
	key := []byte("abcde")
	err = kv.WriteKey(key, ".test", []byte("hello"), 0o600)
	if err != nil {
		return err
	}
	data, err := kv.ReadKey(key, ".test")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	for name, info := range kv.IterKeys(".test") {
		fmt.Println(string(name), info.Size())
	}

	kv, err = store.OpenHomeKV()
	if err != nil {
		return err
	}
	data, err = kv.ReadKey(key, ".test")
	if err != nil {
		return err
	}
	fmt.Println(string(data))

	return nil
}
