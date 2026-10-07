package respconv

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/evgenspm/kclaude/internal/kiroproto"
	tu "github.com/evgenspm/kclaude/internal/testutil"
)

// API-key responses observed on 2026-10-07 contain stopReason in metadataEvent,
// and credits in meteringEvent, without token counts in either event.
func TestUsageFromWireMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, metadata                       string
		input, output, cacheRead, cacheWrite int
	}{
		{"stop reason only", `{"stopReason":"END_TURN"}`, 500, 9, 0, 0},
		{"null token usage", `{"tokenUsage":null}`, 500, 9, 0, 0},
		{"empty token usage", `{"tokenUsage":{}}`, 500, 9, 0, 0},
		{"partial token usage", `{"tokenUsage":{"outputTokens":9}}`, 500, 9, 0, 0},
		{"exact usage", `{"tokenUsage":{"uncachedInputTokens":100,"outputTokens":12,"cacheReadInputTokens":300,"cacheWriteInputTokens":50}}`, 100, 12, 300, 50},
		{"explicit zero usage", `{"tokenUsage":{"uncachedInputTokens":0,"outputTokens":0}}`, 0, 0, 0, 0},
		{"invalid usage", `{"tokenUsage":{"uncachedInputTokens":-1,"outputTokens":12}}`, 500, 9, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var wire bytes.Buffer
			wire.Write(tu.BuildFrame("reasoningContentEvent", []byte(`{"text":"Thinking"}`)))
			wire.Write(tu.BuildFrame("assistantResponseEvent", []byte(`{"content":"Done"}`)))
			wire.Write(tu.BuildFrame("toolUseEvent", []byte(`{"name":"Bash","toolUseId":"tool_1","input":"{\"command\":\"echo hello\"}","stop":true}`)))
			wire.Write(tu.BuildFrame("metadataEvent", []byte(tc.metadata)))
			wire.Write(tu.BuildFrame("contextUsageEvent", []byte(`{"contextUsagePercentage":0.67}`)))
			wire.Write(tu.BuildFrame("meteringEvent", []byte(`{"unit":"credit","unitPlural":"credits","usage":0.0265}`)))
			w := httptest.NewRecorder()
			sw := NewSSEWriter(context.Background(), w, "claude-opus-5-5", 1000000, nil, 0, 500)
			ns := NewNonStreamingAccumulator(1000000, nil, 0, 500)
			err := kiroproto.ParseStream(context.Background(), &wire, func(e kiroproto.Event) bool {
				ns.ProcessEvent(e)
				return sw.HandleEvent(e)
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := sw.Finish(); err != nil {
				t.Fatal(err)
			}
			res, stats := ns.BuildResponse("claude-opus-5-5")
			check := func(label string, usage map[string]any) {
				t.Helper()
				for key, want := range map[string]int{"input_tokens": tc.input, "output_tokens": tc.output, "cache_read_input_tokens": tc.cacheRead, "cache_creation_input_tokens": tc.cacheWrite} {
					if got := usage[key]; got != want {
						t.Errorf("%s %s = %v, want %d", label, key, got, want)
					}
				}
			}
			check("nonstream", res["usage"].(map[string]any))
			found := false
			for _, line := range strings.Split(w.Body.String(), "\n") {
				if !strings.HasPrefix(line, "data: ") {
					continue
				}
				var e struct {
					Type  string         `json:"type"`
					Usage map[string]int `json:"usage"`
				}
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &e); err != nil {
					t.Fatal(err)
				}
				if e.Type != "message_delta" {
					continue
				}
				found = true
				usage := map[string]any{}
				for k, v := range e.Usage {
					usage[k] = v
				}
				check("stream", usage)
			}
			if !found {
				t.Fatal("missing message_delta")
			}
			if !stats.HasCredits || stats.Credits != 0.0265 {
				t.Fatalf("credits lost: %+v", stats)
			}
		})
	}
}
