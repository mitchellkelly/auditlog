package mux

import (
	"net/http"
)

type testingResponseWriter struct {
	header       http.Header
	responseCode int
	responseText []byte
}

func (self testingResponseWriter) Header() http.Header {
	if self.header == nil {
		self.header = http.Header{}
	}

	return self.header
}

func (self *testingResponseWriter) Write(d []byte) (int, error) {
	if self.responseText == nil {
		self.responseText = make([]byte, 0)
	}

	self.responseText = append(self.responseText, d...)

	return len(d), nil
}

func (self *testingResponseWriter) WriteHeader(code int) {
	self.responseCode = code
}
