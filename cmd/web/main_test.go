package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBasicAuth(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	cases := []struct {
		name, user, pass string
		setAuth          bool
		want             int
	}{
		{"no header", "", "", false, http.StatusUnauthorized},
		{"wrong pass", "kaan", "nope", true, http.StatusUnauthorized},
		{"wrong user", "x", "secret", true, http.StatusUnauthorized},
		{"right", "kaan", "secret", true, http.StatusOK},
	}
	h := basicAuth(ok, "kaan", "secret")
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		if c.setAuth {
			r.SetBasicAuth(c.user, c.pass)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("%s: got %d, want %d", c.name, w.Code, c.want)
		}
	}

	w := httptest.NewRecorder()
	basicAuth(ok, "", "").ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != http.StatusOK {
		t.Errorf("unset credentials should disable auth, got %d", w.Code)
	}
}
