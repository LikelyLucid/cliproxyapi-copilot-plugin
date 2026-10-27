package main

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// Reject duplicate or case-variant identity fields; encoding/json otherwise
// accepts both and silently lets their ordering select a credential.
func validQuotaStorageIdentity(raw []byte) bool {
	d := json.NewDecoder(bytes.NewReader(raw))
	start, err := d.Token()
	if err != nil || start != json.Delim('{') {
		return false
	}
	seen := map[string]bool{}
	for d.More() {
		token, err := d.Token()
		key, ok := token.(string)
		if err != nil || !ok {
			return false
		}
		folded := strings.ToLower(key)
		switch folded {
		case "type", "github_login", "github_user_id", "github_access_token":
			if seen[folded] || key != folded {
				return false
			}
			seen[folded] = true
		}
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return false
		}
	}
	end, err := d.Token()
	if err != nil || end != json.Delim('}') {
		return false
	}
	_, err = d.Token()
	return err == io.EOF
}
