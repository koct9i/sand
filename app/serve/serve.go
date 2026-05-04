package serve

import (
	"context"
	"net"
	"time"

	"net/http"

	"github.com/koct9i/sand/rpc"
	"github.com/koct9i/sand/rpc/rest"
)

func NewServer() *http.Server {
	mux := http.NewServeMux()
	rest.RegisterHandler(mux, "/admin/", &rpc.AdminHandler{Admin: rpc.LocalAdmin})
	return &http.Server{
		Handler: &rest.HttpLogger{
			Next: mux,
		},
		ReadHeaderTimeout: time.Second * 120,
	}
}

func Main(ctx context.Context, address string) error {
	srv := NewServer()
	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", address)
	if err != nil {
		return err
	}
	return srv.Serve(ln)
}
