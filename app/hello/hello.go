package hello

import (
	"context"
	"fmt"
)

func Main(ctx context.Context, args []string) error {
	fmt.Println("Hello", args)
	return nil
}
