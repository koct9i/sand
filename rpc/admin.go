package rpc

import (
	"os"
)

//go:generate go run ./gen .

// +sand:rpc
type Admin interface {
	Hostname() (string, error)
	Getpid() (int, error)
}

type localAdmin struct {
}

var _ Admin = (*localAdmin)(nil)

func (a *localAdmin) Hostname() (string, error) {
	return os.Hostname()
}

func (a *localAdmin) Getpid() (int, error) {
	return os.Getpid(), nil
}

var LocalAdmin Admin = &localAdmin{}
