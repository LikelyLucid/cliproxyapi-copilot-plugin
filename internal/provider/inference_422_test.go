package provider

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arthur-sommer-etc/cliproxyapi-copilot-plugin/internal/transport"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

// httpTestHost routes plugin callbacks to a real local HTTP server.
type httpTestHost struct {
	client  *http.Client
	stream  io.ReadCloser
	closed  bool
	emitted bool
}

func (h *httpTestHost) request(ctx context.Context, r transport.Request) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, r.Method, r.URL, bytes.NewReader(r.Body))
	if err != nil {
		return nil, err
	}
	req.Header = r.Headers.Clone()
	return h.client.Do(req)
}
func (h *httpTestHost) Do(ctx context.Context, _ string, r transport.Request) (transport.Response, error) {
	resp, err := h.request(ctx, r)
	if err != nil {
		return transport.Response{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return transport.Response{StatusCode: resp.StatusCode, Headers: resp.Header, Body: body}, err
}
func (h *httpTestHost) OpenStream(ctx context.Context, _ string, r transport.Request) (transport.Stream, error) {
	resp, err := h.request(ctx, r)
	if err != nil {
		return transport.Stream{}, err
	}
	h.stream = resp.Body
	return transport.Stream{ID: "dummy-stream", StatusCode: resp.StatusCode, Headers: resp.Header}, nil
}
func (h *httpTestHost) ReadStream(_ context.Context, _ string) (transport.StreamChunk, error) {
	b := make([]byte, 16)
	n, err := h.stream.Read(b)
	if err != nil && err != io.EOF {
		return transport.StreamChunk{}, err
	}
	return transport.StreamChunk{Payload: b[:n], Done: err == io.EOF}, nil
}
func (h *httpTestHost) CloseStream(_ context.Context, _ string) error {
	h.closed = true
	return h.stream.Close()
}
func (h *httpTestHost) Emit(context.Context, string, []byte) error  { h.emitted = true; return nil }
func (h *httpTestHost) CloseOutput(context.Context, string, string) { h.emitted = true }

func TestInferenceUpstream422NonstreamAndStream(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", streaming), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/copilot_internal/v2/token":
					fmt.Fprint(w, `{"token":"dummy-copilot-secret"}`)
				case "/models":
					fmt.Fprint(w, `{"data":[{"id":"test-model","supported_endpoints":["/responses"]}]}`)
				case "/responses":
					calls++
					body, _ := io.ReadAll(r.Body)
					if got := strings.Contains(string(body), `"stream":true`); got != streaming {
						t.Errorf("upstream stream flag: body=%s", body)
					}
					w.WriteHeader(http.StatusUnprocessableEntity)
					fmt.Fprint(w, `{"error":"invalid input dummy-copilot-secret dummy-github-secret"}`)
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			host := &httpTestHost{client: server.Client()}
			svc := New(host)
			if err := svc.Configure([]byte(fmt.Sprintf("github_api_url: %s\ncopilot_api_url: %s\nallow_insecure_base_urls: true\n", server.URL, server.URL))); err != nil {
				t.Fatal(err)
			}
			req := ExecuteRequest{ExecutorRequest: pluginapi.ExecutorRequest{SourceFormat: "openai-response", Model: "test-model", StorageJSON: []byte(`{"type":"copilot","github_access_token":"dummy-github-secret"}`), Payload: []byte(`{"model":"test-model","input":"hello"}`)}, StreamID: "output"}
			var err error
			if streaming {
				_, err = svc.ExecuteStream(context.Background(), req)
			} else {
				_, err = svc.Execute(context.Background(), req)
			}
			var status *StatusError
			if !errors.As(err, &status) {
				t.Fatalf("expected status error, got %v", err)
			}
			if status.HTTPStatus != 422 || status.Code != "upstream_error" || status.Retryable {
				t.Fatalf("unexpected status: %+v", status)
			}
			if !strings.Contains(status.Message, "invalid input") || strings.Contains(status.Message, "dummy-copilot-secret") || strings.Contains(status.Message, "dummy-github-secret") {
				t.Fatalf("incorrect error message: %s", status.Message)
			}
			if calls != 1 {
				t.Errorf("upstream calls = %d, want 1", calls)
			}
			if streaming && (!host.closed || host.emitted) {
				t.Errorf("stream closed=%v output emitted=%v", host.closed, host.emitted)
			}
		})
	}
}
