package rest

import (
	"log"
	"net/http"
)

type HttpLogger struct {
	Next http.Handler
}

var _ http.Handler = (*HttpLogger)(nil)

func (l *HttpLogger) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	log.Print("reqeust started", r.Method, r.URL.Path)
	l.Next.ServeHTTP(rw, r)
	log.Print("reqeust complete", r.Method, r.URL.Path)
}
