// NHA HTTPS compatibility reference server for limited, authorized testing.
// Designed for Render Free. HTTPS terminates at Render's public edge.
// This is not a speed-test benchmark: traffic is explicitly capped.
package main

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxRequestBytes  int64 = 2 << 20  // The NHA client uses up to 2 MiB per throughput request.
	totalBudgetBytes int64 = 64 << 20 // Budget per direction, per server process lifetime.
	maxConcurrent          = 24
)

type referenceServer struct {
	mu         sync.Mutex
	downloaded int64
	uploaded   int64
	payload    []byte
	slots      chan struct{}
}

func newReferenceServer() (*referenceServer, error) {
	payload := make([]byte, maxRequestBytes)
	if _, err := rand.Read(payload); err != nil {
		return nil, fmt.Errorf("random payload: %w", err)
	}
	return &referenceServer{payload: payload, slots: make(chan struct{}, maxConcurrent)}, nil
}

func (s *referenceServer) reserve(download bool, n int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if download {
		if n > totalBudgetBytes-s.downloaded {
			return false
		}
		s.downloaded += n
		return true
	}
	if n > totalBudgetBytes-s.uploaded {
		return false
	}
	s.uploaded += n
	return true
}

func (s *referenceServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, no-transform")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if r.URL.Path == "/health" && r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "NHA reference server: ready (traffic limited)\n")
		return
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		http.Error(w, "too many concurrent requests", http.StatusTooManyRequests)
		return
	}
	switch {
	case r.URL.Path == "/__down" && r.Method == http.MethodGet:
		s.download(w, r)
	case r.URL.Path == "/__up" && r.Method == http.MethodPost:
		s.upload(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *referenceServer) download(w http.ResponseWriter, r *http.Request) {
	if len(r.URL.Query()) != 1 || len(r.URL.Query()["bytes"]) != 1 {
		http.Error(w, "expected one bytes parameter", http.StatusBadRequest)
		return
	}
	n, err := strconv.ParseInt(r.URL.Query().Get("bytes"), 10, 64)
	if err != nil || n < 1 || n > maxRequestBytes {
		http.Error(w, "bytes must be between 1 and 2097152", http.StatusBadRequest)
		return
	}
	if !s.reserve(true, n) {
		http.Error(w, "download budget exhausted; stop testing", http.StatusTooManyRequests)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(n, 10))
	w.Header().Set("Content-Encoding", "identity")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(s.payload[:int(n)])
}

func (s *referenceServer) upload(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength < 0 || r.ContentLength > maxRequestBytes || r.ContentLength == 0 {
		http.Error(w, "known Content-Length required (1..2097152)", http.StatusRequestEntityTooLarge)
		return
	}
	if !s.reserve(false, r.ContentLength) {
		http.Error(w, "upload budget exhausted; stop testing", http.StatusTooManyRequests)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	n, err := io.Copy(io.Discard, r.Body)
	if err != nil || n != r.ContentLength {
		http.Error(w, "upload was not fully received", http.StatusBadRequest)
		return
	}
	// A receipt is only generated after the exact promised count was read.
	w.Header().Set("X-NHA-Bytes-Received", strconv.FormatInt(n, 10))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "OK\n")
}

func main() {
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "10000"
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		panic(errors.New("PORT must be a valid port"))
	}
	s, err := newReferenceServer()
	if err != nil {
		panic(err)
	}
	httpServer := &http.Server{
		Addr:              ":" + port,
		Handler:           http.HandlerFunc(s.serveHTTP),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       20 * time.Second,
		MaxHeaderBytes:    1 << 16,
	}
	fmt.Printf("NHA test server listening on :%s (64 MiB per direction budget)\n", port)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		panic(err)
	}
}
