package linear

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// --- SlugifyBranch ---

func TestSlugifyBranch(t *testing.T) {
	t.Parallel()
	cases := []struct {
		identifier string
		title      string
		want       string
	}{
		{"ENG-123", "Add JWT auth", "eng-123-add-jwt-auth"},
		{"DX-42", "Fix   multiple   spaces", "dx-42-fix-multiple-spaces"},
		{"ENG-1", "UPPERCASE TITLE", "eng-1-uppercase-title"},
		{"ENG-9", "trailing-", "eng-9-trailing"},
		{"ENG-9", "special chars: @#$%!", "eng-9-special-chars"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.identifier+"/"+tc.title, func(t *testing.T) {
			t.Parallel()
			got := SlugifyBranch(tc.identifier, tc.title)
			if got != tc.want {
				t.Errorf("SlugifyBranch(%q, %q) = %q, want %q", tc.identifier, tc.title, got, tc.want)
			}
		})
	}
}

func TestSlugifyBranch_LongTitle_Truncated(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("a", 100)
	got := SlugifyBranch("ENG-1", long)
	if len(got) > 50 {
		t.Errorf("expected len <= 50, got %d: %q", len(got), got)
	}
	if strings.HasSuffix(got, "-") {
		t.Errorf("slug should not end with dash, got %q", got)
	}
}

// --- PlanFromComment ---

func TestPlanFromComment_Found(t *testing.T) {
	t.Parallel()
	content := "# Ticket: ENG-1\n\n## Objective\nDo the thing\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{
			"data": map[string]any{
				"issue": map[string]any{
					"comments": map[string]any{
						"nodes": []any{
							map[string]any{"id": "c1", "body": "<!-- looper-plan -->\n" + content},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	got, ok, err := client.PlanFromComment(context.Background(), "issue-uuid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected plan to be found")
	}
	if got != content {
		t.Errorf("plan mismatch\ngot:  %q\nwant: %q", got, content)
	}
}

func TestPlanFromComment_NotFound(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{
			"data": map[string]any{
				"issue": map[string]any{
					"comments": map[string]any{
						"nodes": []any{
							map[string]any{"id": "c1", "body": "just a regular comment"},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	_, ok, err := client.PlanFromComment(context.Background(), "issue-uuid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected no plan found")
	}
}

func TestPlanFromComment_Empty(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{
			"data": map[string]any{
				"issue": map[string]any{
					"comments": map[string]any{"nodes": []any{}},
				},
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	_, ok, err := client.PlanFromComment(context.Background(), "issue-uuid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected false for empty comments")
	}
}

func TestPlanFromComment_GraphQLError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{
			"errors": []any{map[string]any{"message": "unauthorized"}},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	_, _, err := client.PlanFromComment(context.Background(), "issue-uuid")
	if err == nil {
		t.Fatal("expected error for GraphQL error response")
	}
}

// --- HTTP client helpers ---

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// newTestClient returns a *Client wired to the given httptest server.
func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c := New("test-key")
	c.httpClient = srv.Client()
	c.baseURL = srv.URL
	return c
}

// --- GetIssue ---

func TestGetIssue_HappyPath(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{
			"data": map[string]any{
				"issue": map[string]any{
					"id":          "uuid-1",
					"identifier":  "ENG-42",
					"title":       "Implement OAuth",
					"description": "Add OAuth 2.0 support",
					"branchName":  "eng-42-implement-oauth",
					"state":       map[string]any{"id": "s1", "name": "Todo", "type": "unstarted"},
					"team":        map[string]any{"id": "team-1"},
					"attachments": map[string]any{"nodes": []any{}},
				},
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	issue, err := client.GetIssue(context.Background(), "ENG-42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issue.ID != "uuid-1" {
		t.Errorf("ID = %q, want %q", issue.ID, "uuid-1")
	}
	if issue.Identifier != "ENG-42" {
		t.Errorf("Identifier = %q, want %q", issue.Identifier, "ENG-42")
	}
	if issue.Title != "Implement OAuth" {
		t.Errorf("Title = %q, want %q", issue.Title, "Implement OAuth")
	}
	if issue.BranchName != "eng-42-implement-oauth" {
		t.Errorf("BranchName = %q, want %q", issue.BranchName, "eng-42-implement-oauth")
	}
	if issue.Team.ID != "team-1" {
		t.Errorf("Team.ID = %q, want %q", issue.Team.ID, "team-1")
	}
}

func TestGetIssue_MinimalFields(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{
			"data": map[string]any{
				"issue": map[string]any{
					"id": "uuid-2", "identifier": "ENG-1", "title": "T", "description": "", "branchName": "",
					"state": map[string]any{"id": "s1", "name": "Todo", "type": "unstarted"},
					"team": map[string]any{"id": "team-1"},
				},
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	issue, err := client.GetIssue(context.Background(), "ENG-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issue.ID != "uuid-2" {
		t.Errorf("ID = %q, want %q", issue.ID, "uuid-2")
	}
}

func TestGetIssue_NotFound(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Linear returns data.issue = null for unknown identifiers.
		writeJSON(w, 200, map[string]any{"data": map[string]any{"issue": nil}})
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	_, err := client.GetIssue(context.Background(), "NOPE-999")
	if err == nil {
		t.Fatal("expected error for missing issue")
	}
}

func TestGetIssue_GraphQLError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{
			"errors": []any{map[string]any{"message": "unauthorized"}},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	_, err := client.GetIssue(context.Background(), "ENG-1")
	if err == nil {
		t.Fatal("expected error for GraphQL error response")
	}
	if !strings.Contains(err.Error(), "unauthorized") {
		t.Errorf("expected 'unauthorized' in error, got: %v", err)
	}
}

func TestGetIssue_HTTPError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte("Unauthorized"))
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	_, err := client.GetIssue(context.Background(), "ENG-1")
	if err == nil {
		t.Fatal("expected error for HTTP 401")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected status code in error, got: %v", err)
	}
}

// --- SetInProgress ---

func TestSetInProgress_HappyPath(t *testing.T) {
	t.Parallel()

	// Track request count to distinguish findStartedState from updateIssueState.
	// atomic.Int32 avoids a data race between the handler goroutine and the test goroutine.
	var reqCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch reqCount.Add(1) {
		case 1: // findStartedState
			writeJSON(w, 200, map[string]any{
				"data": map[string]any{
					"workflowStates": map[string]any{
						"nodes": []any{
							map[string]any{"id": "state-backlog", "name": "Backlog", "type": "backlog"},
							map[string]any{"id": "state-wip", "name": "In Progress", "type": "started"},
						},
					},
				},
			})
		case 2: // updateIssueState
			writeJSON(w, 200, map[string]any{
				"data": map[string]any{
					"issueUpdate": map[string]any{"success": true},
				},
			})
		}
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	if err := client.SetInProgress(context.Background(), "issue-uuid", "team-uuid"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n := reqCount.Load(); n != 2 {
		t.Errorf("expected 2 requests, got %d", n)
	}
}

func TestSetInProgress_NoStartedState(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{
			"data": map[string]any{
				"workflowStates": map[string]any{
					"nodes": []any{
						map[string]any{"id": "state-backlog", "name": "Backlog", "type": "backlog"},
					},
				},
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	err := client.SetInProgress(context.Background(), "issue-uuid", "team-uuid")
	if err == nil {
		t.Fatal("expected error when no started state exists")
	}
	if !strings.Contains(err.Error(), "started") {
		t.Errorf("expected 'started' in error, got: %v", err)
	}
}

// --- CommentPlan ---

func TestCommentPlan_HappyPath(t *testing.T) {
	t.Parallel()
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, 200, map[string]any{
			"data": map[string]any{
				"commentCreate": map[string]any{"success": true},
			},
		})
	}))
	defer srv.Close()

	content := "# Ticket: ENG-1\n\n## Objective\nDo the thing\n"
	client := newTestClient(t, srv)
	if err := client.CommentPlan(context.Background(), "issue-uuid", content); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the comment body contains the marker and the plan content.
	vars, _ := gotBody["variables"].(map[string]any)
	body, _ := vars["body"].(string)
	if !strings.HasPrefix(body, "<!-- looper-plan -->") {
		t.Errorf("body does not start with marker: %q", body)
	}
	if !strings.Contains(body, content) {
		t.Errorf("body does not contain plan content: %q", body)
	}
}

func TestCommentPlan_SuccessFalse(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{
			"data": map[string]any{
				"commentCreate": map[string]any{"success": false},
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	err := client.CommentPlan(context.Background(), "issue-uuid", "plan content")
	if err == nil {
		t.Fatal("expected error when success=false")
	}
	if !strings.Contains(err.Error(), "success=false") {
		t.Errorf("expected 'success=false' in error, got: %v", err)
	}
}

func TestCommentPlan_HTTPError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte("Forbidden"))
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	err := client.CommentPlan(context.Background(), "issue-uuid", "plan content")
	if err == nil {
		t.Fatal("expected error for HTTP 403")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("expected status code in error, got: %v", err)
	}
}

func TestCommentPlan_GraphQLError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{
			"errors": []any{map[string]any{"message": "Entity not found"}},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	err := client.CommentPlan(context.Background(), "issue-uuid", "plan content")
	if err == nil {
		t.Fatal("expected error for GraphQL errors array")
	}
	if !strings.Contains(err.Error(), "Entity not found") {
		t.Errorf("expected error message in output, got: %v", err)
	}
}

func TestSetInProgress_UpdateFails(t *testing.T) {
	t.Parallel()
	var reqCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch reqCount.Add(1) {
		case 1:
			writeJSON(w, 200, map[string]any{
				"data": map[string]any{
					"workflowStates": map[string]any{
						"nodes": []any{
							map[string]any{"id": "state-wip", "name": "In Progress", "type": "started"},
						},
					},
				},
			})
		case 2:
			writeJSON(w, 200, map[string]any{
				"data": map[string]any{
					"issueUpdate": map[string]any{"success": false},
				},
			})
		}
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	err := client.SetInProgress(context.Background(), "issue-uuid", "team-uuid")
	if err == nil {
		t.Fatal("expected error when issueUpdate returns success=false")
	}
}
