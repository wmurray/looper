package linear

import (
	"bytes"
	"context"
	"encoding/base64"
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
	Attachments []Attachment
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

// Attachment is a Linear issue attachment.
type Attachment struct {
	ID    string
	Title string
	URL   string
}

// GetIssue fetches an issue by its identifier (e.g. "ENG-123") or UUID.
func (c *Client) GetIssue(ctx context.Context, identifier string) (*Issue, error) {
	query := `
query($id: String!) {
  issue(id: $id) {
    id identifier title description branchName
    state { id name type }
    team { id }
    attachments { nodes { id title url } }
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
				Attachments struct {
					Nodes []struct {
						ID    string `json:"id"`
						Title string `json:"title"`
						URL   string `json:"url"`
					} `json:"nodes"`
				} `json:"attachments"`
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

	issue := &Issue{
		ID:          raw.ID,
		Identifier:  raw.Identifier,
		Title:       raw.Title,
		Description: raw.Description,
		BranchName:  raw.BranchName,
		State:       WorkflowState{ID: raw.State.ID, Name: raw.State.Name, Type: raw.State.Type},
		Team:        Team{ID: raw.Team.ID},
	}
	for _, a := range raw.Attachments.Nodes {
		issue.Attachments = append(issue.Attachments, Attachment{ID: a.ID, Title: a.Title, URL: a.URL})
	}
	return issue, nil
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

// PlanFromAttachment looks for a looper plan embedded in an issue attachment.
// It looks for an attachment whose title contains "looper-plan" (case-insensitive)
// and whose URL is a base64-encoded data URI: data:text/plain;base64,<content>.
// Returns the decoded plan content and true if found.
func PlanFromAttachment(attachments []Attachment) (string, bool) {
	const prefix = "data:text/plain;base64,"
	for _, a := range attachments {
		if !strings.Contains(strings.ToLower(a.Title), "looper-plan") {
			continue
		}
		if !strings.HasPrefix(a.URL, prefix) {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(a.URL[len(prefix):])
		if err != nil {
			continue
		}
		return string(decoded), true
	}
	return "", false
}

// AttachPlan creates a looper-plan attachment on the issue embedding the plan
// content as a base64 data URI. Call only when the issue has no existing plan
// attachment (i.e. PlanFromAttachment returned false).
func (c *Client) AttachPlan(ctx context.Context, issueID, content string) error {
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	dataURI := "data:text/plain;base64," + encoded

	query := `
mutation($input: AttachmentCreateInput!) {
  attachmentCreate(input: $input) {
    success
  }
}`
	var resp struct {
		Data struct {
			AttachmentCreate struct {
				Success bool `json:"success"`
			} `json:"attachmentCreate"`
		} `json:"data"`
	}

	if err := c.do(ctx, query, map[string]any{
		"input": map[string]any{
			"issueId": issueID,
			"title":   "looper-plan",
			"url":     dataURI,
		},
	}, &resp); err != nil {
		return fmt.Errorf("attachmentCreate: %w", err)
	}
	if !resp.Data.AttachmentCreate.Success {
		return fmt.Errorf("attachmentCreate returned success=false")
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
