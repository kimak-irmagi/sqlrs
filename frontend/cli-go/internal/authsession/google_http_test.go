package authsession

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Direct exchange/refresh assertions moved to the installation transport and server adapter.
func TestProviderRevokerSendsOnlyToken(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Method != "POST" || len(r.Form) != 1 || r.Form.Get("token") != "refresh" {
			t.Error("wrong secret-free revocation")
		}
	}))
	defer s.Close()
	if err := (ProviderRevoker{}).Revoke(context.Background(), s.URL, "refresh"); err != nil {
		t.Fatal(err)
	}
}
func TestProviderRevokerErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad token", http.StatusBadRequest)
	}))
	defer server.Close()

	client := ProviderRevoker{}
	err := client.Revoke(context.Background(), server.URL, "refresh")
	if err == nil || !strings.Contains(err.Error(), "status 400") {
		t.Fatalf("expected revoke status error, got %v", err)
	}

	if err := (ProviderRevoker{}).Revoke(context.Background(), "http://%zz", "refresh"); err == nil {
		t.Fatalf("expected bad revoke endpoint error")
	}
	if err := (ProviderRevoker{HTTP: errorHTTPClient{}}).Revoke(context.Background(), "https://provider.example/revoke", "refresh"); err == nil || !strings.Contains(err.Error(), "http failed") {
		t.Fatalf("expected revoke http error, got %v", err)
	}
}

type errorHTTPClient struct{}

func (errorHTTPClient) Do(*http.Request) (*http.Response, error) {
	return nil, errors.New("http failed")
}

type staticHTTPClient struct {
	resp *http.Response
}

func (c staticHTTPClient) Do(*http.Request) (*http.Response, error) {
	return c.resp, nil
}

type errorReadCloser struct{}

func (errorReadCloser) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

func (errorReadCloser) Close() error {
	return nil
}

var _ io.ReadCloser = errorReadCloser{}
