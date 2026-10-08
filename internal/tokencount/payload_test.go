package tokencount

import (
	"strings"
	"testing"
)

func TestCountKiroPayloadDoesNotTokenizeImageBytes(t *testing.T) {
	request := func(data string) []byte {
		return []byte(`{"conversationState":{"currentMessage":{"userInputMessage":{"content":"describe","images":[{"format":"png","source":{"bytes":"` + data + `"}}]}},"history":[{"userInputMessage":{"content":"earlier","images":[{"format":"jpeg","source":{"bytes":"` + data + `"}}]}}]}}`)
	}
	small, err := CountKiroPayload(request("cGljdHVyZQ=="))
	if err != nil {
		t.Fatal(err)
	}
	largeData := request(strings.Repeat("abc123+/", 128*1024))
	before := string(largeData)
	large, err := CountKiroPayload(largeData)
	if err != nil {
		t.Fatal(err)
	}
	if small != large || small < 2*estimatedImageTokens {
		t.Fatalf("small=%d large=%d", small, large)
	}
	if string(largeData) != before {
		t.Fatal("serialized request was changed")
	}
	plain := []byte(`{"conversationState":{"currentMessage":{"userInputMessage":{"content":"hello"}}}}`)
	want, _ := CountBytes(plain)
	got, err := CountKiroPayload(plain)
	if err != nil || got != want {
		t.Fatalf("text-only count changed: got=%d want=%d err=%v", got, want, err)
	}
	if _, err := CountKiroPayload([]byte(`{broken`)); err == nil {
		t.Fatal("malformed payload accepted")
	}
}
