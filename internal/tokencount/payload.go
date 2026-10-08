package tokencount

import (
	"encoding/json/v2"

	"github.com/evgenspm/kclaude/internal/kiroproto"
)

// Image usage is approximate when Kiro omits token counts. Use a fixed allowance
// per image rather than treating compressed base64 bytes as language tokens.
const estimatedImageTokens = 1600

// CountKiroPayload counts serialized requests, excluding image bytes from BPE.
// It operates on a decoded copy so the request sent upstream remains intact.
func CountKiroPayload(data []byte) (int, error) {
	var payload kiroproto.Payload
	if err := json.Unmarshal(data, &payload); err != nil {
		return 0, err
	}
	images := 0
	strip := func(items []kiroproto.Image) {
		for i := range items {
			items[i].Source.Bytes = ""
			images++
		}
	}
	strip(payload.ConversationState.CurrentMessage.UserInputMessage.Images)
	for _, entry := range payload.ConversationState.History {
		if entry.UserInputMessage != nil {
			strip(entry.UserInputMessage.Images)
		}
	}
	if images == 0 {
		return CountBytes(data)
	}
	text, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	n, err := CountBytes(text)
	return n + images*estimatedImageTokens, err
}
