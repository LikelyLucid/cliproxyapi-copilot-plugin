package main

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestQuotaRejectsAmbiguousStorageIdentity(t *testing.T) {
	for _, storage := range []string{
		`{"github_access_token":"alice","github_access_token":"bob"}`,
		`{"github_access_token":"alice","GitHub_Access_Token":"bob"}`,
	} {
		called := false
		client := quotaTestTransport{quotaRoundTripFunc(func(*http.Request) (*http.Response, error) { called = true; return nil, errors.New("unexpected") })}
		_, err := fetchQuota(context.Background(), quotaFetchRequest{StorageJSON: []byte(storage)}, client)
		if err == nil || called {
			t.Errorf("ambiguous storage must fail before network, err=%v called=%v", err, called)
		}
	}
}
