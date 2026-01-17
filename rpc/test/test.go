package test

import (
	"context"
)

//go:generate go run ../gen .

type Struct struct {
	Field int
}

// +sand:rpc
type Test interface {
	WithNothing()
	WithParam(arg int)
	WithResult() int
	WithVariadic(args ...int)
	WithContext(ctx context.Context)
	WithError() error
	WithStructParam(arg Struct)
	WithStructResult() Struct
	WithNames(ctx_ context.Context, param_ int) (result_ int, err_ error)
}
