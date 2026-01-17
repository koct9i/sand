package serve

import (
	"context"
	"net"
	"time"

	"net/http"

	"github.com/koct9i/sand/rpc"
	"github.com/koct9i/sand/rpc/rest"
)

func Main(ctx context.Context, address string) error {
	mux := http.NewServeMux()
	rest.RegisterHandler(mux, "/admin/", &rpc.AdminHandler{Admin: rpc.LocalAdmin})
	srv := &http.Server{
		Handler: &rest.HttpLogger{
			Next: mux,
		},
		ReadHeaderTimeout: time.Second * 120,
	}
	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", address)
	if err != nil {
		return err
	}
	return srv.Serve(ln)
}
