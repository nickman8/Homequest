package widget

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("secret-token in URL")
}
func TestTransportRedactsErrors(t *testing.T) {
	c := NewClient("secret-token", &http.Client{Transport: failingTransport{}})
	err := c.Call(context.Background(), "getMe", map[string]any{}, nil)
	if err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("unsafe error")
	}
}
func TestBudgets(t *testing.T) {
	if err := Validate([]byte("<p>Дом</p>"), `{"title":"Дом"}`, "refresh"); err != nil {
		t.Fatal(err)
	}
	for _, s := range []struct{ html, state, request string }{
		{strings.Repeat("я", 17000), "", ""}, {"", strings.Repeat("я", 513), ""}, {"", "", strings.Repeat("я", 33)},
	} {
		if Validate([]byte(s.html), s.state, s.request) == nil {
			t.Fatal("oversized payload accepted")
		}
	}
}

type statusTransport struct{ code int }

func (s statusTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: s.code, Body: io.NopCloser(strings.NewReader(`{"parameters":{"retry_after":9},"description":"secret-token"}`)), Header: make(http.Header)}, nil
}
func TestStatusIsStructuredWithoutDescription(t *testing.T) {
	c := NewClient("secret-token", &http.Client{Transport: statusTransport{429}})
	err := c.Call(context.Background(), "getUpdates", nil, nil)
	var api *APIError
	if !errors.As(err, &api) || api.Code != 429 || api.RetryAfter != 9 {
		t.Fatal("missing retry information")
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Fatal("leak")
	}
}
