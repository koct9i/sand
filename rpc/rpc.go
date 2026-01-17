package rpc

import (
	"context"
	"fmt"
	"iter"
)

type Stream interface {
	Send(ctx context.Context, msg any) error
	Recv(ctx context.Context, msg any) error
	Close() error
}

type MethodFunc func(ctx context.Context, stream Stream) error

type Handler interface {
	Methods() iter.Seq2[string, MethodFunc]
	Serve(ctx context.Context, method string, stream Stream) error
}

type Caller interface {
	Call(ctx context.Context, method string, param any, result any) error
	Stream(ctx context.Context, method string) (Stream, error)
}

func UnknownMethod(method string) error {
	return fmt.Errorf("unknown method: %q", method)
}
