package server

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/andybalholm/brotli"
)

const (
	minCompressBytes  = 1024
	largeBodyBytes    = 128 << 10
	brotliSmallLevel  = 4
	brotliLargeLevel  = 1
	encodingBrotli    = "br"
	encodingGzip      = "gzip"
	acceptEncodingKey = "Accept-Encoding"
)

type bufferedResponse struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (b *bufferedResponse) WriteHeader(status int) { b.status = status }

func (b *bufferedResponse) Write(p []byte) (int, error) { return b.body.Write(p) }

func compressResponses(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", acceptEncodingKey)
		encoding := preferredEncoding(r.Header.Get(acceptEncodingKey))
		if encoding == "" {
			next.ServeHTTP(w, r)
			return
		}
		buffered := &bufferedResponse{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(buffered, r)
		body := buffered.body.Bytes()
		header := w.Header()
		if len(body) < minCompressBytes || header.Get("Content-Encoding") != "" || !strings.HasPrefix(header.Get("Content-Type"), "application/json") {
			w.WriteHeader(buffered.status)
			_, _ = w.Write(body)
			return
		}
		var compressed bytes.Buffer
		if err := encode(&compressed, encoding, body); err != nil {
			w.WriteHeader(buffered.status)
			_, _ = w.Write(body)
			return
		}
		header.Set("Content-Encoding", encoding)
		header.Set("Content-Length", strconv.Itoa(compressed.Len()))
		w.WriteHeader(buffered.status)
		_, _ = w.Write(compressed.Bytes())
	})
}

func encode(dst io.Writer, encoding string, body []byte) error {
	var writer io.WriteCloser
	if encoding == encodingBrotli {
		level := brotliSmallLevel
		if len(body) > largeBodyBytes {
			level = brotliLargeLevel
		}
		writer = brotli.NewWriterLevel(dst, level)
	} else {
		writer = gzip.NewWriter(dst)
	}
	if _, err := writer.Write(body); err != nil {
		return err
	}
	return writer.Close()
}

func preferredEncoding(accept string) string {
	gzipAccepted := false
	for part := range strings.SplitSeq(accept, ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if strings.ReplaceAll(strings.TrimSpace(params), " ", "") == "q=0" {
			continue
		}
		switch strings.TrimSpace(name) {
		case encodingBrotli:
			return encodingBrotli
		case encodingGzip:
			gzipAccepted = true
		}
	}
	if gzipAccepted {
		return encodingGzip
	}
	return ""
}
