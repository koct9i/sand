package hello

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
)

func Main(ctx context.Context, args []string) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info("Hello", "args", args)
	fmt.Printf("Hello %#v\n", args)
	return nil
}
