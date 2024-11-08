package server

import (
	"net/http"
)

type handlerSwitcher struct {
	handler              http.Handler
	contentTypeToHandler map[string]http.Handler
}

var _ http.Handler = (*handlerSwitcher)(nil)

func (s *handlerSwitcher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Switch on the content-type to determine which way was used
	// (i.e. gRPC or plain-text HTTP).
	if ch, ok := s.contentTypeToHandler[r.Header.Get("content-type")]; ok {
		ch.ServeHTTP(w, r)
	}
	s.handler.ServeHTTP(w, r)
}
