package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestCompatibilityAndReceipts(t *testing.T) {
	srv, err := newReferenceServer()
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(http.HandlerFunc(srv.serveHTTP))
	defer ts.Close()
	c := ts.Client()
	for _, n := range []int{1, 8192, 256 << 10, 2 << 20, 8 << 20} {
		r, e := c.Get(fmt.Sprintf("%s/__down?bytes=%d", ts.URL, n))
		if e != nil {
			t.Fatal(e)
		}
		b, e := io.ReadAll(r.Body)
		r.Body.Close()
		if e != nil {
			t.Fatal(e)
		}
		if r.StatusCode != 200 || len(b) != n {
			t.Fatalf("download %d status %d bytes %d", n, r.StatusCode, len(b))
		}
	}
	for _, n := range []int{8192, 256 << 10, 2 << 20, 8 << 20} {
		r, e := c.Post(ts.URL+"/__up", "application/octet-stream", bytes.NewReader(make([]byte, n)))
		if e != nil {
			t.Fatal(e)
		}
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
		if r.StatusCode != 200 || r.Header.Get("X-NHA-Bytes-Received") != strconv.Itoa(n) {
			t.Fatalf("upload %d status %d receipt %q", n, r.StatusCode, r.Header.Get("X-NHA-Bytes-Received"))
		}
	}
}
func TestFailClosedAndLimits(t *testing.T) {
	srv, e := newReferenceServer()
	if e != nil {
		t.Fatal(e)
	}
	ts := httptest.NewServer(http.HandlerFunc(srv.serveHTTP))
	defer ts.Close()
	c := ts.Client()
	r, e := c.Get(ts.URL + "/__down?bytes=8388609")
	if e != nil {
		t.Fatal(e)
	}
	if r.StatusCode != 400 {
		t.Fatalf("oversized download status %d", r.StatusCode)
	}
	r.Body.Close()
	r, e = c.Post(ts.URL+"/__up", "application/octet-stream", bytes.NewReader(make([]byte, 8<<20+1)))
	if e != nil {
		t.Fatal(e)
	}
	if r.StatusCode == 200 || r.Header.Get("X-NHA-Bytes-Received") != "" {
		t.Fatalf("oversized upload acknowledged: %d", r.StatusCode)
	}
	r.Body.Close()
	if !srv.reserve(true, totalBudgetBytes-1) {
		t.Fatal("cannot reserve remaining")
	}
	if srv.reserve(true, 2) {
		t.Fatal("budget exceeded")
	}
}
