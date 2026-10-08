package messages

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestLargerThanFourMiB(t *testing.T) {
	text := strings.Repeat("x", 5<<20)
	body := `{"model":"claude-opus-5-5","messages":[{"role":"user","content":"` + text + `"}]}`
	r := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(body))
	req, err := parseAndValidateRequest(context.Background(), httptest.NewRecorder(), r)
	if err != nil {
		t.Fatal(err)
	}
	if req.Messages[0].Content.Text != text {
		t.Fatal("request content was truncated")
	}
}

func TestRequestErrorsReturnUsefulStatus(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	service := &Service{}
	for _, level := range []slog.Level{slog.LevelInfo, slog.LevelDebug} {
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: level})))
		for _, handler := range []http.HandlerFunc{service.HandleMessages, service.HandleCountTokens} {
			for _, tc := range []struct {
				body    string
				status  int
				message string
			}{
				{`{"messages":[{"role":"user","content":"` + strings.Repeat("x", maxRequestBodyBytes) + `"}]}`, 413, "request_too_large"},
				{`{broken`, 400, "invalid_request_error"},
			} {
				r := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(tc.body))
				r.ContentLength = -1 // The reader must enforce the cap for streamed bodies too.
				w := httptest.NewRecorder()
				handler(w, r)
				if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.message) {
					t.Fatalf("level=%s: status=%d body=%s", level, w.Code, w.Body.String())
				}
			}
		}
	}
}
