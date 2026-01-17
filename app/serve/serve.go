package serve

import (
	"context"
	"net"

	"net/http"

	"github.com/koct9i/sand/rpc"
	"github.com/koct9i/sand/rpc/rest"
)

func Main(ctx context.Context, address string) error {
	mux := http.NewServeMux()
	rest.RegisterHandler(mux, "/admin/", &rpc.AdminHandler{
		Admin: rpc.LocalAdmin,
	})
	srv := &http.Server{
		Handler: mux,
	}
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	return srv.Serve(ln)
}
