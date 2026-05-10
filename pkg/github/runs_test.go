package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFetchRecentRuns_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/Integration-Project-2026-Groep-2/Kassa/actions/runs", r.URL.Path)
		assert.Equal(t, "main", r.URL.Query().Get("head_branch"))
		assert.Equal(t, "5", r.URL.Query().Get("per_page"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"workflow_runs":[
			{"head_sha":"abc123","created_at":"2026-05-10T14:18:03Z","conclusion":"success","name":"CD","html_url":"https://github.com/x/y/actions/runs/1"},
			{"head_sha":"deadbeef","created_at":"2026-05-10T13:00:00Z","conclusion":"success","name":"CD","html_url":"https://github.com/x/y/actions/runs/2"}
		]}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	runs, err := c.FetchRecentRuns(context.Background(), "Integration-Project-2026-Groep-2/Kassa", 5)

	assert.NoError(t, err)
	assert.Len(t, runs, 2)
	assert.Equal(t, "abc123", runs[0].HeadSHA)
	assert.Equal(t, "success", runs[0].Conclusion)
	assert.Equal(t, "CD", runs[0].WorkflowName)
	assert.Equal(t, time.Date(2026, 5, 10, 14, 18, 3, 0, time.UTC), runs[0].CreatedAt)
}

func TestFetchRecentRuns_404Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Not Found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	_, err := c.FetchRecentRuns(context.Background(), "x/y", 5)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestFetchRecentRuns_DefaultLimitWhenZero(t *testing.T) {
	var seenPerPage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPerPage = r.URL.Query().Get("per_page")
		_, _ = w.Write([]byte(`{"workflow_runs":[]}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	_, _ = c.FetchRecentRuns(context.Background(), "x/y", 0)
	assert.Equal(t, "5", seenPerPage)
}

func TestFetchRecentRuns_CapsLimitAt30(t *testing.T) {
	var seenPerPage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPerPage = r.URL.Query().Get("per_page")
		_, _ = w.Write([]byte(`{"workflow_runs":[]}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	_, _ = c.FetchRecentRuns(context.Background(), "x/y", 100)
	assert.Equal(t, "5", seenPerPage)
}

func TestFetchRecentRuns_AuthHeaderWhenTokenSet(t *testing.T) {
	var seenAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"workflow_runs":[]}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client(), Token: "ghp_test123"}
	_, _ = c.FetchRecentRuns(context.Background(), "x/y", 5)
	assert.True(t, strings.HasPrefix(seenAuth, "Bearer "))
	assert.Contains(t, seenAuth, "ghp_test123")
}

func TestFetchRecentRuns_NoAuthHeaderWhenTokenEmpty(t *testing.T) {
	var seenAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"workflow_runs":[]}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client(), Token: ""}
	_, _ = c.FetchRecentRuns(context.Background(), "x/y", 5)
	assert.Empty(t, seenAuth)
}
