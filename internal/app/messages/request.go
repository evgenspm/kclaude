package messages

// Modified by ClaudeCode Kiro to count native WebSearch requests safely.

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/evgenspm/kclaude/internal/anthropic"
	"github.com/evgenspm/kclaude/internal/httpx"
	"github.com/evgenspm/kclaude/internal/models"
	"github.com/evgenspm/kclaude/internal/reqconv"
	"github.com/evgenspm/kclaude/internal/tokencount"
)

// Allow long Claude Code conversations with base64 image results. This matches
// the outer kclaude-router limit; a smaller inner limit rejects valid requests.
const maxRequestBodyBytes = 64 << 20

func writeRequestError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "request_too_large",
			"request body exceeds the 64 MiB limit; compact the conversation or use smaller images")
		return
	}
	httpx.WriteError(w, http.StatusBadRequest, errTypeInvalidRequest, err.Error())
}

// HandleCountTokens serves POST /v1/messages/count_tokens.
func (s *Service) HandleCountTokens(w http.ResponseWriter, r *http.Request) {
	req, err := parseAndValidateRequest(r.Context(), w, r)
	if err != nil {
		writeRequestError(w, err)
		return
	}

	profileARN := ""
	if creds, err := s.auth.GetToken(r.Context()); err == nil {
		profileARN = creds.ProfileARN
	} else {
		slog.DebugContext(r.Context(), "count_tokens proceeding without credentials", "err", err)
	}

	kiroModel, thinking, _, _ := models.Resolve(req.Model, anthropic.HasContext1MBeta(r.Header))
	if req.IsThinkingEnabled() {
		thinking = true
	}

	ccSessionID := r.Header.Get(headerCCSessionID)
	if hasNativeWebSearch(req) {
		n := estimateWebSearchTokens(req)
		httpx.WriteJSON(w, http.StatusOK, map[string]int{"input_tokens": n})
		return
	}

	// Mirror the live send path so token counts include effort (envState is
	// derived inside BuildPayload from the system prompt).
	effort := resolveEffort(r.Context(), kiroModel, req, thinking)

	payload, _, err := reqconv.BuildPayload(req, reqconv.BuildOptions{ProfileARN: profileARN, ModelID: kiroModel, ConversationID: ccSessionID, Effort: effort})
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, errTypeInvalidRequest, err.Error())
		return
	}

	data, err := json.Marshal(payload)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, errTypeAPI, "failed to serialize payload")
		return
	}

	n, err := tokencount.CountKiroPayload(data)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, errTypeAPI, "token counting unavailable")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.MarshalWrite(w, map[string]int{"input_tokens": n}); err != nil {
		slog.ErrorContext(r.Context(), "write count_tokens response failed", "err", err)
		return
	}
	_, _ = w.Write([]byte("\n"))
}

// parseAndValidateRequest decodes and validates an Anthropic request from the HTTP body.
func parseAndValidateRequest(ctx context.Context, w http.ResponseWriter, r *http.Request) (*anthropic.Request, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	var req anthropic.Request
	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, fmt.Errorf("invalid request: %w", err)
		}
		slog.DebugContext(ctx, "client request body", "request_body", jsontext.Value(raw))
		if err := json.UnmarshalDecode(jsontext.NewDecoder(bytes.NewReader(raw)), &req); err != nil {
			return nil, fmt.Errorf("invalid request: %w", err)
		}
	} else {
		if err := json.UnmarshalRead(r.Body, &req); err != nil {
			return nil, fmt.Errorf("invalid request: %w", err)
		}
	}
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("messages must not be empty")
	}
	return &req, nil
}
