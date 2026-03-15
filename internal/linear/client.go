package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

const apiURL = "https://api.linear.app/graphql"

// Client is a minimal Linear GraphQL client authenticated with a personal API key.
type Client struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string // overridable for testing; defaults to apiURL
}

// New creates a Client using the given personal API key.
func New(apiKey string) *Client {
	return &Client{apiKey: apiKey, httpClient: &http.Client{}, baseURL: apiURL}
}

// Issue holds the fields from Linear needed by looper start.
type Issue struct {
	ID          string
	Identifier  string
	Title       string
	Description string
	BranchName  string
	State       WorkflowState
	Team        Team
}

// WorkflowState is a Linear workflow state (backlog, started, completed, etc.).
type WorkflowState struct {
	ID   string
	Name string
	Type string
}

// Team is the Linear team the issue belongs to.
type Team struct {
	ID string
}

// GetIssue fetches an issue by its identifier (e.g. "ENG-123") or UUID.
func (c *Client) GetIssue(ctx context.Context, identifier string) (*Issue, error) {
	query := `
query($id: String!) {
  issue(id: $id) {
    id identifier title description branchName
    state { id name type }
    team { id }
  }
}`
	var resp struct {
		Data struct {
			Issue struct {
				ID          string `json:"id"`
				Identifier  string `json:"identifier"`
				Title       string `json:"title"`
				Description string `json:"description"`
				BranchName  string `json:"branchName"`
				State       struct {
					ID   string `json:"id"`
					Name string `json:"name"`
					Type string `json:"type"`
				} `json:"state"`
				Team struct {
					ID string `json:"id"`
				} `json:"team"`
			} `json:"issue"`
		} `json:"data"`
	}

	if err := c.do(ctx, query, map[string]any{"id": identifier}, &resp); err != nil {
		return nil, err
	}

	raw := resp.Data.Issue
	if raw.ID == "" {
		return nil, fmt.Errorf("issue %q not found", identifier)
	}

	return &Issue{
		ID:          raw.ID,
		Identifier:  raw.Identifier,
		Title:       raw.Title,
		Description: raw.Description,
		BranchName:  raw.BranchName,
		State:       WorkflowState{ID: raw.State.ID, Name: raw.State.Name, Type: raw.State.Type},
		Team:        Team{ID: raw.Team.ID},
	}, nil
}

// SetInProgress finds the first "started" workflow state for the issue's team
// and updates the issue to that state.
func (c *Client) SetInProgress(ctx context.Context, issueID, teamID string) error {
	stateID, err := c.findStartedState(ctx, teamID)
	if err != nil {
		return fmt.Errorf("find In Progress state: %w", err)
	}
	return c.updateIssueState(ctx, issueID, stateID)
}

func (c *Client) findStartedState(ctx context.Context, teamID string) (string, error) {
	query := `
query($teamId: ID!) {
  workflowStates(filter: { team: { id: { eq: $teamId } } }) {
    nodes { id name type }
  }
}`
	var resp struct {
		Data struct {
			WorkflowStates struct {
				Nodes []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
					Type string `json:"type"`
				} `json:"nodes"`
			} `json:"workflowStates"`
		} `json:"data"`
	}
	if err := c.do(ctx, query, map[string]any{"teamId": teamID}, &resp); err != nil {
		return "", err
	}
	for _, s := range resp.Data.WorkflowStates.Nodes {
		if s.Type == "started" {
			return s.ID, nil
		}
	}
	return "", fmt.Errorf("no 'started' workflow state found for team %s", teamID)
}

func (c *Client) updateIssueState(ctx context.Context, issueID, stateID string) error {
	query := `
mutation($id: String!, $stateId: String!) {
  issueUpdate(id: $id, input: { stateId: $stateId }) {
    success
  }
}`
	var resp struct {
		Data struct {
			IssueUpdate struct {
				Success bool `json:"success"`
			} `json:"issueUpdate"`
		} `json:"data"`
	}
	if err := c.do(ctx, query, map[string]any{"id": issueID, "stateId": stateID}, &resp); err != nil {
		return err
	}
	if !resp.Data.IssueUpdate.Success {
		return fmt.Errorf("issueUpdate returned success=false")
	}
	return nil
}

const planCommentMarker = "<!-- looper-plan -->"

// PlanFromComment queries the issue's comments by UUID for a looper plan comment.
// Returns the plan body (with the marker line stripped), true if found, or an error.
// A comment whose body is exactly the marker with no content is treated as absent.
func (c *Client) PlanFromComment(ctx context.Context, issueID string) (string, bool, error) {
	query := `
query($id: String!) {
  issue(id: $id) {
    comments(first: 250) { nodes { id body } }
  }
}`
	var resp struct {
		Data struct {
			Issue struct {
				Comments struct {
					Nodes []struct {
						ID   string `json:"id"`
						Body string `json:"body"`
					} `json:"nodes"`
				} `json:"comments"`
			} `json:"issue"`
		} `json:"data"`
	}
	if err := c.do(ctx, query, map[string]any{"id": issueID}, &resp); err != nil {
		return "", false, err
	}
	for _, n := range resp.Data.Issue.Comments.Nodes {
		if strings.HasPrefix(n.Body, planCommentMarker) {
			content := strings.TrimPrefix(n.Body, planCommentMarker)
			content = strings.TrimPrefix(content, "\n")
			if content == "" {
				continue // marker-only comment with no plan body
			}
			return content, true, nil
		}
	}
	return "", false, nil
}

// CommentPlan posts the plan as a comment on the issue with a looper-plan marker.
// Call only when PlanFromComment returned false (no existing plan comment).
func (c *Client) CommentPlan(ctx context.Context, issueID, content string) error {
	query := `
mutation($issueId: String!, $body: String!) {
  commentCreate(input: { issueId: $issueId, body: $body }) {
    success
  }
}`
	var resp struct {
		Data struct {
			CommentCreate struct {
				Success bool `json:"success"`
			} `json:"commentCreate"`
		} `json:"data"`
	}
	body := planCommentMarker + "\n" + content
	if err := c.do(ctx, query, map[string]any{"issueId": issueID, "body": body}, &resp); err != nil {
		return err
	}
	if !resp.Data.CommentCreate.Success {
		return fmt.Errorf("commentCreate returned success=false")
	}
	return nil
}

// branchSlugRe matches any run of characters that are not lowercase ASCII alphanumeric.
var branchSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

// SlugifyBranch creates a git-safe branch name from an identifier and title.
// Used as a fallback when issue.BranchName is empty.
// The slug contains only [a-z0-9-] so byte-level truncation at 50 is safe.
func SlugifyBranch(identifier, title string) string {
	slug := strings.ToLower(identifier + "-" + title)
	slug = branchSlugRe.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if len(slug) > 50 {
		slug = slug[:50]
		slug = strings.TrimRight(slug, "-")
	}
	return slug
}

// graphqlRequest is the JSON body sent to the Linear API.
type graphqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type graphqlError struct {
	Message string `json:"message"`
}

// do executes a GraphQL query/mutation and unmarshals the full response into out.
// out must be a pointer to a struct with a Data field matching the response shape.
func (c *Client) do(ctx context.Context, query string, variables map[string]any, out any) error {
	body, err := json.Marshal(graphqlRequest{Query: query, Variables: variables})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("linear API status %d: %s", resp.StatusCode, string(raw))
	}

	// Surface GraphQL-level errors before decoding the data payload.
	var errCheck struct {
		Errors []graphqlError `json:"errors"`
	}
	if err := json.Unmarshal(raw, &errCheck); err == nil && len(errCheck.Errors) > 0 {
		msgs := make([]string, len(errCheck.Errors))
		for i, e := range errCheck.Errors {
			msgs[i] = e.Message
		}
		return fmt.Errorf("linear API error: %s", strings.Join(msgs, "; "))
	}

	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
