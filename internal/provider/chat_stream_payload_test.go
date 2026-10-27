package provider

import (
	"bytes"
	"testing"
)

func TestChatStreamPayload(t *testing.T) {
	cases := []struct {
		frame, want string
		wantErr     bool
	}{
		{"data: {\"id\":\"test\"}\n\n", "{\"id\":\"test\"}", false},
		{"data: [DONE]\n\n", "", false},
		{": keepalive\n\n", "", false},
		{"data: not-json\n\n", "", true},
	}
	for _, tc := range cases {
		got, err := chatStreamPayload([]byte(tc.frame))
		if (err != nil) != tc.wantErr || !bytes.Equal(got, []byte(tc.want)) {
			t.Errorf("frame %q got %q err %v", tc.frame, got, err)
		}
	}
}
