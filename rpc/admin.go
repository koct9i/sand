package rpc

import (
	"os"
)

//go:generate go run ./gen .

// +sand:rpc
type Admin interface {
	Hostname() (hostname string, err error)
	Getpid() (pid int, err error)
	Loadavg() (load1, load5, load15 float64)
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
