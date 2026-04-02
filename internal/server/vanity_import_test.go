package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type mockResolver struct {
	urls map[string]string
}

func (m *mockResolver) CloneURL(name string) string {
	return m.urls[name]
}

func TestHandleVanityImport(t *testing.T) {
	resolver := &mockResolver{
		urls: map[string]string{
			"samverk":  "http://192.168.1.160:3000/samverk/samverk.git",
			"synapset": "http://192.168.1.160:3000/samverk-admin/synapset.git",
		},
	}
	handler := handleVanityImport(resolver)

	tests := []struct {
		name       string
		path       string
		query      string
		wantStatus int
		wantMeta   string
	}{
		{
			name:       "samverk go-get",
			path:       "/samverk",
			query:      "go-get=1",
			wantStatus: http.StatusOK,
			wantMeta:   `content="samverk.dev/samverk git http://192.168.1.160:3000/samverk/samverk.git"`,
		},
		{
			name:       "synapset go-get",
			path:       "/synapset",
			query:      "go-get=1",
			wantStatus: http.StatusOK,
			wantMeta:   `content="samverk.dev/synapset git http://192.168.1.160:3000/samverk-admin/synapset.git"`,
		},
		{
			name:       "sub-path resolves to root project",
			path:       "/samverk/internal/version",
			query:      "go-get=1",
			wantStatus: http.StatusOK,
			wantMeta:   `content="samverk.dev/samverk git http://192.168.1.160:3000/samverk/samverk.git"`,
		},
		{
			name:       "unknown project",
			path:       "/unknown",
			query:      "go-get=1",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "no go-get param falls through",
			path:       "/samverk",
			query:      "",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "empty path",
			path:       "/",
			query:      "go-get=1",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := tt.path
			if tt.query != "" {
				url += "?" + tt.query
			}
			req := httptest.NewRequest(http.MethodGet, url, http.NoBody)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			if tt.wantMeta != "" {
				body := rec.Body.String()
				if !strings.Contains(body, tt.wantMeta) {
					t.Errorf("body missing meta tag %q\ngot: %s", tt.wantMeta, body)
				}
			}
		})
	}
}
