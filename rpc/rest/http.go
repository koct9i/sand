package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/koct9i/sand/rpc"
)

type Caller struct {
	http.Client
	BaseURL string
}

var _ rpc.Caller = (*Caller)(nil)

func (c *Caller) Call(ctx context.Context, method string, param any, result any) error {
	httpMethod := http.MethodGet
	var buf *bytes.Buffer
	if param != nil {
		httpMethod = http.MethodPost
		buf = bytes.NewBuffer(nil)
		if err := json.NewEncoder(buf).Encode(param); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, httpMethod, c.BaseURL+method, buf)
	if err != nil {
		return err
	}
	rsp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("server response: %s", rsp.Status)
	}
	if result != nil {
		if err := json.NewDecoder(rsp.Body).Decode(result); err != nil {
			return err
		}
	}
	return nil
}

func (c *Caller) Stream(ctx context.Context, method string) (rpc.Stream, error) {
	return nil, nil
}

type jsonStream struct {
	rc io.ReadCloser
	w  io.Writer
}

func (s *jsonStream) Recv(ctx context.Context, msg any) error {
	return json.NewDecoder(s.rc).Decode(msg)
}

func (s *jsonStream) Send(ctx context.Context, msg any) error {
	return json.NewEncoder(s.w).Encode(msg)
}

func (s *jsonStream) Close() error {
	return s.rc.Close()
}

type httpMethodFunc rpc.MethodFunc

func (f httpMethodFunc) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	err := f(r.Context(), &jsonStream{
		rc: r.Body,
		w:  rw,
	})
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
	} else {
		rw.WriteHeader(http.StatusOK)
	}
	r.Body.Close()
}

func RegisterHandler(mux *http.ServeMux, path string, handler rpc.Handler) {
	for method, methodFunc := range handler.Methods() {
		mux.Handle(path+method, httpMethodFunc(methodFunc))
	}
}
