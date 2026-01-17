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
	WithParam(int)
	WithResult() int
	WithVariadic(...int)
	WithContext(context.Context)
	WithError() error
	WithStructParam(Struct)
	WithStructResult() Struct
	WithNames(ctx_ context.Context, param_ int) (result_ int, err_ error)
}
