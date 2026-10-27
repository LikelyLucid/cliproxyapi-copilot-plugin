package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/arthur-sommer-etc/cliproxyapi-copilot-plugin/internal/translate"
	"github.com/arthur-sommer-etc/cliproxyapi-copilot-plugin/internal/transport"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

type fallbackTestHost struct {
	t      *testing.T
	stream bool
	done   chan string
	frames []byte
}

func (h *fallbackTestHost) Do(_ context.Context, _ string, r transport.Request) (transport.Response, error) {
	switch {
	case strings.HasSuffix(r.URL, "/copilot_internal/v2/token"):
		return transport.Response{StatusCode: 200, Body: []byte(`{"token":"test-token"}`)}, nil
	case strings.HasSuffix(r.URL, "/models"):
		return transport.Response{StatusCode: 200, Body: []byte(`{"data":[{"id":"test-chat","capabilities":{"type":"chat"}}]}`)}, nil
	case strings.HasSuffix(r.URL, "/chat/completions"):
		h.checkRequest(r, false)
		return transport.Response{StatusCode: 200, Body: []byte(`{"id":"chatcmpl-1","object":"chat.completion","model":"test-chat","choices":[{"index":0,"message":{"role":"assistant","content":"hello back"},"finish_reason":"stop"}]}`)}, nil
	default:
		return transport.Response{}, fmt.Errorf("unexpected URL: %s", r.URL)
	}
}
func (h *fallbackTestHost) checkRequest(r transport.Request, stream bool) {
	var body map[string]any
	if err := json.Unmarshal(r.Body, &body); err != nil {
		h.t.Errorf("decode request: %v", err)
		return
	}
	if got, _ := body["stream"].(bool); got != stream {
		h.t.Errorf("stream=%v, want %v", got, stream)
	}
	if body["model"] != "test-chat" {
		h.t.Errorf("model = %v", body["model"])
	}
}
func (h *fallbackTestHost) OpenStream(_ context.Context, _ string, r transport.Request) (transport.Stream, error) {
	if !strings.HasSuffix(r.URL, "/chat/completions") {
		return transport.Stream{}, fmt.Errorf("unexpected stream URL: %s", r.URL)
	}
	h.checkRequest(r, true)
	return transport.Stream{ID: "upstream", StatusCode: 200, Headers: http.Header{}}, nil
}
func (h *fallbackTestHost) ReadStream(context.Context, string) (transport.StreamChunk, error) {
	return transport.StreamChunk{Payload: []byte("data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"model\":\"test-chat\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello back\"},\"finish_reason\":null}]}\n\ndata: [DONE]\n\n"), Done: true}, nil
}
func (h *fallbackTestHost) CloseStream(context.Context, string) error { return nil }
func (h *fallbackTestHost) Emit(_ context.Context, _ string, data []byte) error {
	h.frames = append(h.frames, data...)
	return nil
}
func (h *fallbackTestHost) CloseOutput(_ context.Context, _ string, err string) { h.done <- err }

func TestMissingEndpointChatInference(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			host := &fallbackTestHost{t: t, done: make(chan string, 1)}
			svc := New(host)
			req := ExecuteRequest{ExecutorRequest: pluginapi.ExecutorRequest{SourceFormat: "openai-response", Model: "test-chat", StorageJSON: []byte(`{"type":"copilot","github_access_token":"test-secret"}`), Payload: []byte(`{"model":"test-chat","input":"hello"}`)}, StreamID: "output"}
			if stream {
				_, err := svc.ExecuteStream(context.Background(), req)
				if err != nil {
					t.Fatalf("stream: %v", err)
				}
				select {
				case err := <-host.done:
					if err != "" {
						t.Fatalf("stream output: %s", err)
					}
					if !strings.Contains(string(host.frames), "hello back") {
						t.Fatalf("missing output: %s", host.frames)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("stream did not finish")
				}
			} else {
				resp, err := svc.Execute(context.Background(), req)
				if err != nil {
					t.Fatalf("execute: %v", err)
				}
				if !strings.Contains(string(resp.Payload), "hello back") {
					t.Fatalf("missing output: %s", resp.Payload)
				}
				if resp.Metadata["copilot_endpoint"] != translate.EndpointChatCompletions {
					t.Fatalf("endpoint metadata: %v", resp.Metadata)
				}
			}
		})
	}
}

func TestMissingEndpointFallbackOnlyForChat(t *testing.T) {
	cases := []struct {
		name  string
		model upstreamModel
		want  string
	}{
		{"missing chat", upstreamModel{ID: "chat", Capabilities: modelCapabilities{Type: " Chat "}}, translate.EndpointChatCompletions},
		{"empty chat", upstreamModel{ID: "chat", SupportedEndpoints: []string{}, Capabilities: modelCapabilities{Type: "chat"}}, ""},
		{"missing unknown type", upstreamModel{ID: "unknown"}, ""},
		{"missing embeddings", upstreamModel{ID: "embedding", Capabilities: modelCapabilities{Type: "embeddings"}}, ""},
		{"explicit unsupported", upstreamModel{ID: "chat", SupportedEndpoints: []string{"/embeddings"}, Capabilities: modelCapabilities{Type: "chat"}}, ""},
		{"explicit responses", upstreamModel{ID: "chat", SupportedEndpoints: []string{"/responses"}, Capabilities: modelCapabilities{Type: "chat"}}, translate.EndpointResponses},
		{"special model", upstreamModel{ID: "gpt-5.6-sol", Capabilities: modelCapabilities{Type: "chat"}}, translate.EndpointResponses},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			models := normalizeModels([]upstreamModel{tc.model})
			got, err := selectEndpoint(models[0])
			if tc.want == "" {
				if err == nil {
					t.Fatalf("unexpected endpoint %q", got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("endpoint=%q err=%v, want %q", got, err, tc.want)
			}
			if !contains(modelInfos(models)[0].SupportedGenerationMethods, tc.want) {
				t.Fatalf("missing metadata: %#v", modelInfos(models)[0].SupportedGenerationMethods)
			}
		})
	}
}
