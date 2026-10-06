package layasticker

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type originTestTransport func(*http.Request) (*http.Response, error)

func (f originTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestClientRedirectCredentialOrigins(t *testing.T) {
	for _, tc := range []struct {
		name string
		urls []string
		auth []bool
	}{
		{"same source", []string{"https://laya.example/start", "https://laya.example/next"}, []bool{true, true}},
		{"default port", []string{"https://laya.example/start", "https://laya.example:443/next"}, []bool{true, true}},
		{"other host", []string{"https://laya.example/start", "https://other.example/next"}, []bool{true, false}},
		{"changed scheme", []string{"https://laya.example/start", "http://laya.example/next"}, []bool{true, false}},
		{"cross port then same source", []string{"https://laya.example:8000/start", "https://laya.example:8001/hop", "https://laya.example:8001/end"}, []bool{true, false, false}},
		{"return after crossing", []string{"https://laya.example/start", "http://laya.example/hop", "https://laya.example/end"}, []bool{true, false, false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			transport := originTestTransport(func(r *http.Request) (*http.Response, error) {
				i := calls
				calls++
				if i >= len(tc.urls) {
					t.Fatal("unexpected extra request")
				}
				if r.URL.String() != tc.urls[i] {
					t.Errorf("request URL mismatch")
				}
				if (r.Header.Get("Authorization") != "") != tc.auth[i] {
					t.Errorf("hop %d credential presence incorrect", i)
				}
				resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"answers":{"sticker":{"choice":"NONE"}}}`)), Request: r}
				if i+1 < len(tc.urls) {
					resp.StatusCode = http.StatusTemporaryRedirect
					resp.Header.Set("Location", tc.urls[i+1])
				}
				return resp, nil
			})
			cfg := testConfig(tc.urls[0])
			cfg.APIKey = "test-only-key"
			client, err := NewClientWithHTTP(cfg, &http.Client{Transport: transport})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.Decide(context.Background(), DecisionRequest{}); err != nil {
				t.Fatal(err)
			}
			if calls != len(tc.urls) {
				t.Fatal("redirect chain incomplete")
			}
		})
	}
}
