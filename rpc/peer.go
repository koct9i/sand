package rpc

import (
	. "github.com/koct9i/sand/core"
)

type Peer interface {
	ID() UUID
	Name() string
}

type Protocol interface {
	Name() string
}

type Service interface {
}

type Registry interface {
	Register(ctx Context, name string, service Service)
	Discovery(ctx Context, name string) (Service, error)
}
