package config

import (
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	c, err := load(env(map[string]string{"DATABASE_URL": "postgres://x"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.HTTPAddr != ":8080" || c.Env != "development" || c.CookieSecure {
		t.Fatalf("valeurs par défaut inattendues : %+v", c)
	}
}

func TestLoadReportsEveryProblem(t *testing.T) {
	_, err := load(env(map[string]string{"APP_ENV": "nope", "COOKIE_SECURE": "peut-etre", "S3_BUCKET": "b"}))
	if err == nil {
		t.Fatal("erreur attendue")
	}
	for _, want := range []string{"APP_ENV", "DATABASE_URL", "COOKIE_SECURE", "S3_ENDPOINT"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q absent de l'erreur : %v", want, err)
		}
	}
}

func TestProductionRequiresSafeguards(t *testing.T) {
	_, err := load(env(map[string]string{
		"APP_ENV": "production", "DATABASE_URL": "postgres://x", "COOKIE_SECURE": "false",
	}))
	if err == nil {
		t.Fatal("erreur attendue")
	}
	for _, want := range []string{"COOKIE_SECURE", "S3", "HTTPS"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q absent de l'erreur : %v", want, err)
		}
	}
}
