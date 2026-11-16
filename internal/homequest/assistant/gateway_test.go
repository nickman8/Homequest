package assistant

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type captureTransport struct {
	t    *testing.T
	host string
}

func (c captureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host != c.host || r.URL.Path != "/v1/chat/completions" {
		c.t.Fatalf("wrong gateway destination: %s", r.URL.Host)
	}
	if r.Header.Get("Authorization") != "Bearer fixture-key" {
		c.t.Fatal("missing authorization")
	}
	var body map[string]any
	if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
		c.t.Fatal(e)
	}
	if body["model"] != "verified-fixture-model" {
		c.t.Fatal("model substituted")
	}
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"{\"action\":\"complete\",\"ids\":[1]}"}}]}`))}, nil
}
func TestConfiguredGatewayUsesVerifiedModel(t *testing.T) {
	n := &NickLLM{BaseURL: "https://gateway.example/v1/", Key: "fixture-key", Model: "verified-fixture-model", HTTP: &http.Client{Transport: captureTransport{t, "gateway.example"}}}
	c, e := n.Parse(context.Background(), "Я выкинул мусор", []Candidate{{ID: 1, Title: "Вынести мусор", Version: 1}}, "UTC")
	if e != nil || c.Action != "complete" {
		t.Fatal(c, e)
	}
}
func TestUnsafeGatewayConfigurationRejected(t *testing.T) {
	for _, base := range []string{"http://gateway.example/v1", "https://user:password@gateway.example/v1", "https://gateway.example/v1?secret=x", ""} {
		n := &NickLLM{BaseURL: base, Key: "fixture-key", Model: "verified-fixture-model"}
		if _, e := n.Parse(context.Background(), "hello", nil, "UTC"); e == nil {
			t.Fatal("unsafe config accepted")
		}
	}
}
