// Local account pool around the upstream kirocc protocol adapter.
package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/evgenspm/kclaude/internal/auth"
	"github.com/evgenspm/kclaude/internal/kiroclient"
	"github.com/evgenspm/kclaude/internal/kiroproto"
	"github.com/evgenspm/kclaude/internal/server"
	"github.com/evgenspm/kclaude/internal/tokencount"
)

type account struct {
	Name     string `json:"name"`
	KeyFile  string `json:"key_file"`
	Region   string `json:"region"`
	Disabled bool   `json:"disabled,omitempty"`
	key      string
	client   kiroclient.Client
	cooldown time.Time
	calls    int
}
type pool struct {
	mu       sync.Mutex
	accounts []*account
	active   int
}
type seatKey struct{}

// Explicit 1M aliases advertise context without implicitly enabling thinking.
const modelMappings = `[
 {"anthropic":"claude-opus-5-5[1m]","kiro":"claude-opus-5.5","kiro_1m":"claude-opus-5.5"},
 {"anthropic":"claude-sonnet-5-5[1m]","kiro":"claude-sonnet-5.5","kiro_1m":"claude-sonnet-5.5"},
 {"anthropic":"claude-sonnet-4-6[1m]","kiro":"claude-sonnet-4.6","kiro_1m":"claude-sonnet-4.6"},
 {"anthropic":"claude-opus-5-5","kiro":"claude-opus-5.5","kiro_1m":"claude-opus-5.5"},
 {"anthropic":"claude-sonnet-5-5","kiro":"claude-sonnet-5.5","kiro_1m":"claude-sonnet-5.5"},
 {"anthropic":"claude-sonnet-4-6","kiro":"claude-sonnet-4.6","kiro_1m":"claude-sonnet-4.6"}
]`

func (p *pool) pick(tried map[int]bool, pinned string) (int, *account) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for j := 0; j < len(p.accounts); j++ {
		i := (p.active + j) % len(p.accounts)
		a := p.accounts[i]
		if !tried[i] && !a.Disabled && !time.Now().Before(a.cooldown) && (pinned == "" || pinned == a.Name) {
			return i, a
		}
	}
	return -1, nil
}
func (p *pool) GetToken(context.Context) (*auth.Credentials, error) {
	return &auth.Credentials{AccessToken: "pool", Region: "us-east-1", AuthType: auth.AuthTypeAPIKey}, nil
}
func retryDelay(h http.Header) time.Duration {
	if n, e := strconv.Atoi(h.Get("Retry-After")); e == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	if t, e := http.ParseTime(h.Get("Retry-After")); e == nil && time.Until(t) > 0 {
		return time.Until(t)
	}
	return time.Minute
}
func (p *pool) GenerateAssistantResponse(ctx context.Context, _ string, payload *kiroproto.Payload, _ string) (*kiroclient.Response, error) {
	tried := map[int]bool{}
	pinned, _ := ctx.Value(seatKey{}).(string)
	var last *kiroclient.Response
	for range p.accounts {
		i, a := p.pick(tried, pinned)
		if a == nil {
			break
		}
		tried[i] = true
		if last != nil {
			last.Body.Close()
			last = nil
		}
		r, err := a.client.GenerateAssistantResponse(ctx, a.key, payload, a.Region)
		// An ambiguous network failure may have been billed. Do not replay it.
		if err != nil {
			var ue *kiroclient.UpstreamError
			if !errors.As(err, &ue) {
				return nil, err
			}
			status := ue.Status
			if ue.Exception == "ThrottlingException" || ue.Exception == "TooManyRequestsException" {
				status = 429
			}
			if status != 401 && status != 402 && status != 403 && status != 429 {
				return nil, err
			}
			r = &kiroclient.Response{StatusCode: status, Header: ue.Header, Body: io.NopCloser(strings.NewReader(ue.Body))}
		}
		p.mu.Lock()
		a.calls++
		p.mu.Unlock()
		switch r.StatusCode {
		case 401, 402, 403, 429:
			b, e := io.ReadAll(io.LimitReader(r.Body, 1024*1024))
			r.Body.Close()
			if e != nil {
				return nil, e
			}
			r.Body = io.NopCloser(bytes.NewReader(b))
			last = r
			delay := retryDelay(r.Header)
			if r.StatusCode != 429 {
				delay = 15 * time.Minute
			}
			low := strings.ToLower(string(b))
			if strings.Contains(low, "quota") || strings.Contains(low, "credit") || strings.Contains(low, "monthly") {
				delay = max(delay, time.Hour)
			}
			p.mu.Lock()
			a.cooldown = time.Now().Add(delay)
			if pinned == "" {
				p.active = (i + 1) % len(p.accounts)
			}
			p.mu.Unlock()
			slog.Info("Kiro account unavailable", "seat", a.Name, "status", r.StatusCode, "cooldown", delay)
			// Only rejected requests are retried. Never replay a started 200 stream.
		default:
			if r.StatusCode == 200 && pinned == "" {
				p.mu.Lock()
				p.active = i
				p.mu.Unlock()
			}
			return r, nil
		}
	}
	if last != nil {
		b, _ := io.ReadAll(last.Body)
		last.Body.Close()
		return nil, &kiroclient.UpstreamError{Status: last.StatusCode, Header: last.Header, Body: string(b)}
	}
	return nil, &kiroclient.UpstreamError{Status: 429, Body: `{"message":"No Kiro seat ready; run kclaude status. Accounts are cooling down or the selected seat is unavailable."}`}
}
func (p *pool) SearchWeb(ctx context.Context, _, _ string, _ string, q string) (*kiroclient.WebSearchResponse, error) {
	pinned, _ := ctx.Value(seatKey{}).(string)
	_, a := p.pick(map[int]bool{}, pinned)
	if a == nil {
		return nil, errors.New("no Kiro seat ready for search")
	}
	c, ok := a.client.(kiroclient.WebSearchClient)
	if !ok {
		return nil, errors.New("search unavailable")
	}
	return c.SearchWeb(ctx, a.key, "", a.Region, q)
}
func loadPool(path string) (*pool, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	var cfg struct {
		Accounts []*account `json:"accounts"`
	}
	if e = json.Unmarshal(b, &cfg); e != nil {
		return nil, e
	}
	p := &pool{}
	seen := map[string]bool{}
	for _, a := range cfg.Accounts {
		if a == nil {
			return nil, errors.New("invalid null account")
		}
		if a.Disabled {
			continue
		}
		if a.Name == "" || seen[a.Name] {
			return nil, errors.New("seat names must be unique and nonempty")
		}
		seen[a.Name] = true
		if a.Region == "" {
			a.Region = "us-east-1"
		}
		if a.Region != "us-east-1" && a.Region != "eu-central-1" {
			return nil, errors.New("unsupported Kiro region")
		}
		st, e := os.Stat(a.KeyFile)
		if e != nil {
			return nil, fmt.Errorf("%s: key file missing", a.Name)
		}
		if st.Mode().Perm()&0077 != 0 {
			return nil, fmt.Errorf("%s: key file must be chmod 600", a.Name)
		}
		b, e := os.ReadFile(a.KeyFile)
		if e != nil {
			return nil, e
		}
		a.key = strings.TrimSpace(string(b))
		if !strings.HasPrefix(a.key, "ksk_") {
			return nil, fmt.Errorf("%s: expected a Kiro API key", a.Name)
		}
		a.client = kiroclient.NewHTTPClient(kiroclient.WithAPIKeyAuth(), kiroclient.WithNoRetries(), kiroclient.WithRegion(a.Region), kiroclient.WithTokenCounter(tokencount.CountBytes))
		p.accounts = append(p.accounts, a)
	}
	if len(p.accounts) == 0 {
		return nil, errors.New("no enabled Kiro accounts")
	}
	return p, nil
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	state := flag.String("state", "", "private state directory")
	port := flag.Int("port", 17391, "loopback port (1024-65535)")
	flag.Parse()
	if *state == "" {
		return errors.New("--state required")
	}
	if *port < 1024 || *port > 65535 {
		return errors.New("invalid loopback port")
	}
	absState, e := filepath.Abs(*state)
	if e != nil {
		return e
	}
	*state = absState
	p, e := loadPool(filepath.Join(*state, "accounts.json"))
	if e != nil {
		return e
	}
	b, e := os.ReadFile(filepath.Join(*state, "proxy.key"))
	if e != nil {
		return e
	}
	key := strings.TrimSpace(string(b))
	if len(key) < 32 {
		return errors.New("invalid local proxy key")
	}
	certificate, e := localCertificate(*state)
	if e != nil {
		return fmt.Errorf("local TLS certificate: %w", e)
	}
	// New model aliases keep the 1M context advertised to Claude Code.
	os.Setenv("KIROCC_MODEL_MAPPINGS", modelMappings)
	h := server.New(p, key, p, server.WithKeepAliveInterval(15*time.Second)).Handler()
	sig := make(chan os.Signal, 1)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Do not expose credentialled local endpoints to cross-origin browser pages.
		if r.Header.Get("Origin") != "" {
			http.Error(w, "browser origin not allowed", 403)
			return
		}
		if r.URL.Path == "/health" {
			w.Header().Set("Content-Type", "application/json")
			challenge := r.URL.Query().Get("challenge")
			if len(challenge) != 64 {
				http.Error(w, "expected 64-character challenge", 400)
				return
			}
			mac := hmac.New(sha256.New, []byte(key))
			mac.Write([]byte("kclaude-router:" + challenge))
			json.NewEncoder(w).Encode(map[string]any{"service": "kclaude-router", "proof": hex.EncodeToString(mac.Sum(nil))})
			return
		}
		supplied := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if supplied == "" {
			supplied = r.Header.Get("x-api-key")
		}
		if subtle.ConstantTimeCompare([]byte(supplied), []byte(key)) != 1 {
			http.Error(w, "unauthorized", 401)
			return
		}
		if r.URL.Path == "/kclaude/stop" {
			if r.Method != http.MethodPost {
				http.Error(w, "POST required", 405)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"stopping":true}`)
			select {
			case sig <- syscall.SIGTERM:
			default:
			}
			return
		}
		if r.URL.Path == "/kclaude/status" {
			p.mu.Lock()
			defer p.mu.Unlock()
			rows := []map[string]any{}
			for i, a := range p.accounts {
				rows = append(rows, map[string]any{"name": a.Name, "active": i == p.active, "calls": a.calls, "cooldown_seconds": max(0, int(time.Until(a.cooldown).Seconds()))})
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"service": "kclaude-router", "pid": os.Getpid(), "state": *state, "accounts": rows})
			return
		}
		if name := r.Header.Get("X-Kclaude-Seat"); name != "" {
			r = r.WithContext(context.WithValue(r.Context(), seatKey{}, name))
		}
		r.Header.Set("Authorization", "Bearer "+key)
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024*1024)
		h.ServeHTTP(w, r)
	})
	srv := &http.Server{Addr: "127.0.0.1:" + strconv.Itoa(*port), Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second,
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{certificate}}}
	go tokencount.Preload()
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sig)
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		<-sig
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()
	slog.Info("kclaude ready", "address", srv.Addr, "accounts", len(p.accounts))
	e = srv.ListenAndServeTLS("", "")
	if errors.Is(e, http.ErrServerClosed) {
		<-drained
		return nil
	}
	return e
}
