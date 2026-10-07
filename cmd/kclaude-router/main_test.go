package main

import (
	"context"
	"errors"
	"github.com/evgenspm/kclaude/internal/kiroclient"
	"github.com/evgenspm/kclaude/internal/kiroproto"
	"github.com/evgenspm/kclaude/internal/models"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDefaultModelContextDoesNotEnableThinking(t *testing.T) {
	t.Setenv("KIROCC_MODEL_MAPPINGS", modelMappings)
	for _, kind := range []string{"opus", "sonnet"} {
		for _, suffix := range []string{"", "[1m]", "[1M]"} {
			input := "claude-" + kind + "-5-5" + suffix
			kiro, thinking, window, _ := models.Resolve(input, false)
			if kiro != "claude-"+kind+"-5.5" || thinking || window != 1_000_000 {
				t.Fatalf("Resolve(%q) = %q, thinking=%v, context=%d", input, kiro, thinking, window)
			}
		}
	}
}

type mockClient func() (*kiroclient.Response, error)

func (f mockClient) GenerateAssistantResponse(context.Context, string, *kiroproto.Payload, string) (*kiroclient.Response, error) {
	return f()
}

type brokenBody struct{ emitted bool }

func (b *brokenBody) Read(p []byte) (int, error) {
	if b.emitted {
		return 0, errors.New("connection interrupted")
	}
	b.emitted = true
	return copy(p, "partial stream"), nil
}
func (*brokenBody) Close() error { return nil }

func TestNeverReplayAmbiguousFailure(t *testing.T) {
	for _, started := range []bool{false, true} {
		fallbackCalls := 0
		p := &pool{accounts: []*account{
			{Name: "first", client: mockClient(func() (*kiroclient.Response, error) {
				if started {
					return &kiroclient.Response{StatusCode: 200, Body: &brokenBody{}}, nil
				}
				return nil, errors.New("ambiguous transport error")
			})},
			{Name: "fallback", client: mockClient(func() (*kiroclient.Response, error) {
				fallbackCalls++
				return nil, errors.New("unexpected fallback")
			})},
		}}
		r, err := p.GenerateAssistantResponse(t.Context(), "", &kiroproto.Payload{}, "")
		if started {
			if err != nil {
				t.Fatal(err)
			}
			_, err = io.ReadAll(r.Body)
			r.Body.Close()
		}
		if err == nil || fallbackCalls != 0 {
			t.Fatalf("started=%v err=%v fallbackCalls=%d", started, err, fallbackCalls)
		}
	}
}

func testPool(t *testing.T, firstStatus int, body string) (*pool, *int, *int) {
	t.Helper()
	n1, n2 := 0, 0
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n1++
		if r.Header.Get("Authorization") != "Bearer ksk_one" || r.Header.Get("TokenType") != "API_KEY" {
			t.Error("wrong upstream credentials")
		}
		if r.Header.Get("x-amzn-codewhisperer-optout") != "true" {
			t.Error("training opt-out missing")
		}
		w.Header().Set("Retry-After", "123")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(firstStatus)
		io.WriteString(w, body)
	}))
	t.Cleanup(a.Close)
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n2++
		if r.Header.Get("Authorization") != "Bearer ksk_two" {
			t.Error("wrong fallback credential")
		}
		w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
		io.WriteString(w, "stream")
	}))
	t.Cleanup(b.Close)
	return &pool{accounts: []*account{
		{Name: "seat1", key: "ksk_one", client: kiroclient.NewHTTPClient(kiroclient.WithBaseURL(a.URL), kiroclient.WithAPIKeyAuth(), kiroclient.WithNoRetries())},
		{Name: "seat2", key: "ksk_two", client: kiroclient.NewHTTPClient(kiroclient.WithBaseURL(b.URL), kiroclient.WithAPIKeyAuth(), kiroclient.WithNoRetries())},
	}}, &n1, &n2
}
func TestRealTransportFailover(t *testing.T) {
	for _, code := range []int{401, 402, 403, 429} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			p, n1, n2 := testPool(t, code, `{"message":"rejected"}`)
			r, e := p.GenerateAssistantResponse(context.Background(), "", &kiroproto.Payload{}, "")
			if e != nil {
				t.Fatal(e)
			}
			r.Body.Close()
			if *n1 != 1 || *n2 != 1 || p.active != 1 {
				t.Fatalf("calls=%d,%d active=%d", *n1, *n2, p.active)
			}
			if code == 429 && time.Until(p.accounts[0].cooldown) < 120*time.Second {
				t.Fatal("Retry-After not respected")
			}
		})
	}
}
func TestHTTP200ThrottleEnvelopeFailsOver(t *testing.T) {
	p, _, n2 := testPool(t, 200, `{"__type":"ThrottlingException"}`)
	r, e := p.GenerateAssistantResponse(context.Background(), "", &kiroproto.Payload{}, "")
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	if *n2 != 1 {
		t.Fatal("not retried")
	}
}
func TestValidationErrorNotReplayed(t *testing.T) {
	p, n1, n2 := testPool(t, 400, `{"message":"bad tool schema"}`)
	_, e := p.GenerateAssistantResponse(context.Background(), "", &kiroproto.Payload{}, "")
	if e == nil || *n1 != 1 || *n2 != 0 {
		t.Fatalf("%v %d %d", e, *n1, *n2)
	}
}
func TestPinnedSeatNeverSpills(t *testing.T) {
	p, _, n2 := testPool(t, 429, `{"message":"quota"}`)
	_, e := p.GenerateAssistantResponse(context.WithValue(context.Background(), seatKey{}, "seat1"), "", &kiroproto.Payload{}, "")
	var ue *kiroclient.UpstreamError
	if !errors.As(e, &ue) || ue.Status != 429 || *n2 != 0 {
		t.Fatalf("%v, fallback calls %d", e, *n2)
	}
}
func TestNoReadySeats(t *testing.T) {
	p, _, _ := testPool(t, 429, "")
	for _, a := range p.accounts {
		a.cooldown = time.Now().Add(time.Hour)
	}
	_, e := p.GenerateAssistantResponse(context.Background(), "", &kiroproto.Payload{}, "")
	if e == nil || !strings.Contains(e.Error(), "No Kiro seat ready") {
		t.Fatal(e)
	}
}
