package reqconv

import (
	"encoding/json/v2"
	"strings"
	"testing"

	"github.com/evgenspm/kclaude/internal/anthropic"
)

func TestReadImagesSurviveConversion(t *testing.T) {
	const result = `{"role":"user","content":[{"type":"tool_result","tool_use_id":"read1","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"cGljdHVyZQ=="}}]}]}`
	const call = `{"role":"assistant","content":[{"type":"tool_use","id":"read1","name":"Read","input":{}}]}`
	const tools = `,"tools":[{"name":"Read","description":"Read a file","input_schema":{"type":"object"}}]`
	for _, tc := range []struct{ name, messages, tools string }{
		{"current", `{"role":"user","content":"read it"},` + call + `,` + result, tools},
		{"history", `{"role":"user","content":"read it"},` + call + `,` + result + `,{"role":"assistant","content":"loaded"},{"role":"user","content":"what does it show?"}`, tools},
		{"no tools", call + `,` + result, ""},
		{"orphan", result, tools},
		{"assistant ending", call + `,` + result + `,{"role":"assistant","content":"loaded"}`, tools},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var req anthropic.Request
			if err := json.Unmarshal([]byte(`{"model":"claude-opus-5-5","messages":[`+tc.messages+`]`+tc.tools+`}`), &req); err != nil {
				t.Fatal(err)
			}
			payload, _, err := BuildPayload(&req, BuildOptions{ModelID: "claude-opus-5.5"})
			if err != nil {
				t.Fatal(err)
			}
			wire, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(wire), `"bytes":"cGljdHVyZQ=="`) != 1 {
				t.Fatalf("Read image must occur exactly once in payload: %s", wire)
			}
			if strings.Contains(string(wire), "(empty result)") {
				t.Fatalf("image-only Read was described as empty: %s", wire)
			}
			images := payload.ConversationState.CurrentMessage.UserInputMessage.Images
			if tc.name == "history" || tc.name == "assistant ending" {
				if len(images) != 0 {
					t.Fatal("historical image moved into current message")
				}
				for _, h := range payload.ConversationState.History {
					if h.UserInputMessage != nil {
						images = append(images, h.UserInputMessage.Images...)
					}
				}
			}
			if len(images) != 1 || images[0].Source.Bytes != "cGljdHVyZQ==" || images[0].Format != "png" {
				t.Fatalf("image missing from expected protocol field: %+v", images)
			}
		})
	}
}

func TestParallelReadImagesMatchToolResultOrder(t *testing.T) {
	for _, historical := range []bool{false, true} {
		body := `{"tools":[{"name":"Read","input_schema":{"type":"object"}}],"messages":[
		{"role":"user","content":"read both"},
		{"role":"assistant","content":[{"type":"tool_use","name":"Read","id":"a","input":{}},{"type":"tool_use","name":"Read","id":"b","input":{}}]},
		{"role":"user","content":[
		{"type":"tool_result","tool_use_id":"b","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"image-b"}}]},
		{"type":"tool_result","tool_use_id":"a","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"image-a"}}]}]}`
		if historical {
			body += `,{"role":"assistant","content":"loaded"},{"role":"user","content":"compare them"}`
		}
		body += `]}`
		var req anthropic.Request
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			t.Fatal(err)
		}
		before, _ := json.Marshal(req)
		payload, _, err := BuildPayload(&req, BuildOptions{})
		if err != nil {
			t.Fatal(err)
		}
		current := payload.ConversationState.CurrentMessage.UserInputMessage
		images, ctx := current.Images, current.UserInputMessageContext
		if historical {
			msg := payload.ConversationState.History[2].UserInputMessage
			images, ctx = msg.Images, msg.UserInputMessageContext
		}
		if len(images) != 2 || ctx == nil || len(ctx.ToolResults) != 2 {
			t.Fatal("missing images or results")
		}
		for i, id := range []string{"a", "b"} {
			if ctx.ToolResults[i].ToolUseID != id || images[i].Source.Bytes != "image-"+id {
				t.Fatalf("historical=%v: wrong association for %s", historical, id)
			}
		}
		after, _ := json.Marshal(req)
		if string(before) != string(after) {
			t.Fatal("source request was mutated")
		}
	}
}
