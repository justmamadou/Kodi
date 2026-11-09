package e2e

import (
	"net/url"
	"testing"
)

func mustURL(t *testing.T) *url.URL {
	t.Helper()
	u, err := url.Parse(env.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
