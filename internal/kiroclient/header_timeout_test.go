package kiroclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/evgenspm/kclaude/internal/kiroproto"
)

func TestHTTPClient_WaitsForMultimodalResponseHeaders(t *testing.T) {
	// A live request with 19 images needed more than 30 seconds after upload
	// before Kiro sent its headers. Reproduce that boundary, not a short delay
	// that would also pass with the old transport timeout.
	srv := newTCP4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-time.After(31 * time.Second):
			w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
			_, _ = w.Write([]byte("stream-body"))
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	c := NewHTTPClient(WithBaseURL(srv.URL), WithNoRetries())
	resp, err := c.GenerateAssistantResponse(ctx, "test-token", &kiroproto.Payload{}, "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil || string(body) != "stream-body" {
		t.Fatalf("body = %q, error = %v", body, err)
	}
}

func TestHTTPClient_CancelWhileWaitingForResponseHeaders(t *testing.T) {
	started := make(chan struct{})
	srv := newTCP4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := NewHTTPClient(WithBaseURL(srv.URL), WithNoRetries())
	done := make(chan error, 1)
	go func() {
		resp, err := c.GenerateAssistantResponse(ctx, "test-token", &kiroproto.Payload{}, "us-east-1")
		if resp != nil {
			_ = resp.Body.Close()
		}
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not arrive")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not interrupt the header wait")
	}
}
