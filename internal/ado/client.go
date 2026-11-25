package ado

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	apiVersion = "7.1"
	// The comments API is only available as a preview version.
	commentsAPIVersion = "7.1-preview.4"
	// Azure DevOps rejects work item batch reads with more than 200 IDs.
	maxBatchSize   = 200
	defaultTimeout = 30 * time.Second
	maxBodySize    = 32 << 20
)

// Config configures a Client.
type Config struct {
	// OrgURL is the organization name ("contoso") or URL
	// ("https://dev.azure.com/contoso").
	OrgURL string
	// Project is the project name.
	Project string
	// PAT is a personal access token with work item read/write scope.
	// It may be empty for servers that don't need auth, like the mock server.
	PAT string
	// HTTPClient is optional; a client with a 30s timeout is used by default.
	HTTPClient *http.Client
}

// Client talks to the Azure DevOps REST API.
type Client struct {
	orgURL  string
	project string
	auth    string
	http    *http.Client
}

// NewClient validates cfg and returns a client.
func NewClient(cfg Config) (*Client, error) {
	org := strings.TrimRight(strings.TrimSpace(cfg.OrgURL), "/")
	if org == "" {
		return nil, errors.New("organization is required")
	}
	if !strings.Contains(org, "://") {
		org = "https://dev.azure.com/" + url.PathEscape(org)
	}
	if u, err := url.Parse(org); err != nil || u.Host == "" {
		return nil, fmt.Errorf("invalid organization URL %q", cfg.OrgURL)
	}
	project := strings.TrimSpace(cfg.Project)
	if project == "" {
		return nil, errors.New("project is required")
	}

	c := &Client{orgURL: org, project: project, http: cfg.HTTPClient}
	if c.http == nil {
		c.http = &http.Client{Timeout: defaultTimeout}
	}
	if cfg.PAT != "" {
		c.auth = "Basic " + base64.StdEncoding.EncodeToString([]byte(":"+cfg.PAT))
	}
	return c, nil
}

// OrgURL returns the organization URL the client talks to.
func (c *Client) OrgURL() string { return c.orgURL }

// Project returns the project name.
func (c *Client) Project() string { return c.project }

// QueryWorkItems runs the query and returns the matching work items in query
// order.
func (c *Client) QueryWorkItems(ctx context.Context, q Query) ([]WorkItem, error) {
	params := url.Values{}
	if q.Top > 0 {
		params.Set("$top", strconv.Itoa(q.Top))
	}
	var resp struct {
		WorkItems []struct {
			ID int `json:"id"`
		} `json:"workItems"`
	}
	body := map[string]string{"query": q.WIQL()}
	if err := c.do(ctx, http.MethodPost, c.projectURL("wit/wiql", params), "application/json", body, &resp); err != nil {
		return nil, fmt.Errorf("query work items: %w", err)
	}

	ids := make([]int, len(resp.WorkItems))
	for i, wi := range resp.WorkItems {
		ids[i] = wi.ID
	}
	return c.GetWorkItems(ctx, ids)
}

// GetWorkItems reads the given work items, in order. IDs that no longer exist
// are skipped.
func (c *Client) GetWorkItems(ctx context.Context, ids []int) ([]WorkItem, error) {
	items := make([]WorkItem, 0, len(ids))
	for start := 0; start < len(ids); start += maxBatchSize {
		batch := ids[start:min(start+maxBatchSize, len(ids))]
		params := url.Values{
			"ids":         {joinInts(batch)},
			"fields":      {strings.Join(itemFields, ",")},
			"errorPolicy": {"omit"},
		}
		var resp struct {
			Value []*wireWorkItem `json:"value"`
		}
		if err := c.do(ctx, http.MethodGet, c.projectURL("wit/workitems", params), "", nil, &resp); err != nil {
			return nil, fmt.Errorf("get work items: %w", err)
		}
		for _, w := range resp.Value {
			if w != nil { // errorPolicy=omit returns null for missing items
				items = append(items, w.toWorkItem())
			}
		}
	}
	return items, nil
}

// GetWorkItem reads a single work item.
func (c *Client) GetWorkItem(ctx context.Context, id int) (WorkItem, error) {
	params := url.Values{"fields": {strings.Join(itemFields, ",")}}
	var w wireWorkItem
	if err := c.do(ctx, http.MethodGet, c.projectURL("wit/workitems/"+strconv.Itoa(id), params), "", nil, &w); err != nil {
		return WorkItem{}, fmt.Errorf("get work item %d: %w", id, err)
	}
	return w.toWorkItem(), nil
}

// UpdateWorkItem applies changes to a work item and returns the updated item.
// If rev is positive the update only succeeds when the item is still at that
// revision; otherwise IsConflict reports true for the returned error.
func (c *Client) UpdateWorkItem(ctx context.Context, id, rev int, changes []Change) (WorkItem, error) {
	type patchOp struct {
		Op    string `json:"op"`
		Path  string `json:"path"`
		Value any    `json:"value"`
	}
	ops := make([]patchOp, 0, len(changes)+1)
	if rev > 0 {
		ops = append(ops, patchOp{Op: "test", Path: "/rev", Value: rev})
	}
	for _, ch := range changes {
		ops = append(ops, patchOp{Op: "add", Path: "/fields/" + ch.Field, Value: ch.Value})
	}

	var w wireWorkItem
	u := c.projectURL("wit/workitems/"+strconv.Itoa(id), nil)
	if err := c.do(ctx, http.MethodPatch, u, "application/json-patch+json", ops, &w); err != nil {
		return WorkItem{}, fmt.Errorf("update work item %d: %w", id, err)
	}
	return w.toWorkItem(), nil
}

// GetComments returns a work item's comments, newest first.
func (c *Client) GetComments(ctx context.Context, id int) ([]Comment, error) {
	var comments []Comment
	token := ""
	for {
		params := url.Values{
			"api-version": {commentsAPIVersion},
			"$top":        {"200"},
			"order":       {"desc"},
		}
		if token != "" {
			params.Set("continuationToken", token)
		}
		var resp struct {
			Comments          []Comment `json:"comments"`
			ContinuationToken string    `json:"continuationToken"`
		}
		u := c.projectURL("wit/workitems/"+strconv.Itoa(id)+"/comments", params)
		if err := c.do(ctx, http.MethodGet, u, "", nil, &resp); err != nil {
			return nil, fmt.Errorf("get comments for %d: %w", id, err)
		}
		comments = append(comments, resp.Comments...)
		if resp.ContinuationToken == "" || len(resp.Comments) == 0 {
			break
		}
		token = resp.ContinuationToken
	}
	sort.SliceStable(comments, func(i, j int) bool {
		return comments[i].CreatedDate.After(comments[j].CreatedDate)
	})
	return comments, nil
}

// AddComment posts a plain-text comment to a work item.
func (c *Client) AddComment(ctx context.Context, id int, text string) (Comment, error) {
	params := url.Values{"api-version": {commentsAPIVersion}}
	body := map[string]string{"text": TextToHTML(text)}
	var comment Comment
	u := c.projectURL("wit/workitems/"+strconv.Itoa(id)+"/comments", params)
	if err := c.do(ctx, http.MethodPost, u, "application/json", body, &comment); err != nil {
		return Comment{}, fmt.Errorf("add comment to %d: %w", id, err)
	}
	return comment, nil
}

// GetStates returns the workflow states of a work item type.
func (c *Client) GetStates(ctx context.Context, t WorkItemType) ([]State, error) {
	var resp struct {
		Value []State `json:"value"`
	}
	u := c.projectURL("wit/workitemtypes/"+url.PathEscape(string(t))+"/states", nil)
	if err := c.do(ctx, http.MethodGet, u, "", nil, &resp); err != nil {
		return nil, fmt.Errorf("get states for %s: %w", t, err)
	}
	return resp.Value, nil
}

// GetTeamMembers returns the members of a team, sorted by name. An empty
// team name means the project's default team ("<project> Team").
func (c *Client) GetTeamMembers(ctx context.Context, team string) ([]Identity, error) {
	if team == "" {
		team = c.project + " Team"
	}
	u := fmt.Sprintf("%s/_apis/projects/%s/teams/%s/members?%s",
		c.orgURL, url.PathEscape(c.project), url.PathEscape(team),
		url.Values{"api-version": {apiVersion}, "$top": {"1000"}}.Encode())
	var resp struct {
		Value []struct {
			Identity Identity `json:"identity"`
		} `json:"value"`
	}
	if err := c.do(ctx, http.MethodGet, u, "", nil, &resp); err != nil {
		return nil, fmt.Errorf("get members of team %q: %w", team, err)
	}
	members := make([]Identity, 0, len(resp.Value))
	for _, m := range resp.Value {
		members = append(members, m.Identity)
	}
	sort.Slice(members, func(i, j int) bool {
		return strings.ToLower(members[i].Name()) < strings.ToLower(members[j].Name())
	})
	return members, nil
}

// projectURL builds a project-scoped API URL. api-version defaults to 7.1.
func (c *Client) projectURL(path string, params url.Values) string {
	if params == nil {
		params = url.Values{}
	}
	if params.Get("api-version") == "" {
		params.Set("api-version", apiVersion)
	}
	return c.orgURL + "/" + url.PathEscape(c.project) + "/_apis/" + path + "?" + params.Encode()
}

// do sends a request with a JSON body (if body is non-nil) and decodes the
// JSON response into out (if out is non-nil).
func (c *Client) do(ctx context.Context, method, rawURL, contentType string, body, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", contentType)
	}
	if c.auth != "" {
		req.Header.Set("Authorization", c.auth)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return newAPIError(resp.StatusCode, data)
	}
	// Azure DevOps answers bad credentials with a 203 and an HTML sign-in
	// page rather than a 401, so check that we actually got JSON.
	if mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type")); mt != "application/json" {
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    "unexpected non-JSON response; check the organization URL and personal access token",
		}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// APIError is a non-success response from Azure DevOps.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s (HTTP %d)", e.Message, e.StatusCode)
}

// IsConflict reports whether err means the work item was changed by someone
// else since it was read. Azure DevOps reports that as TF26071; the status
// code is also checked in case the message differs.
func IsConflict(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.StatusCode == http.StatusConflict ||
		apiErr.StatusCode == http.StatusPreconditionFailed ||
		strings.Contains(apiErr.Message, "TF26071")
}

func newAPIError(status int, body []byte) *APIError {
	var payload struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &payload) == nil && payload.Message != "" {
		return &APIError{StatusCode: status, Message: payload.Message}
	}

	msg := http.StatusText(status)
	switch status {
	case http.StatusUnauthorized:
		msg = "authentication failed; check your personal access token"
	case http.StatusForbidden:
		msg = "access denied; check your personal access token's scopes"
	case http.StatusNotFound:
		msg = "not found; check the organization and project names"
	}
	return &APIError{StatusCode: status, Message: msg}
}

// wireWorkItem is a work item as returned by the REST API.
type wireWorkItem struct {
	ID     int            `json:"id"`
	Rev    int            `json:"rev"`
	Fields map[string]any `json:"fields"`
}

func (w *wireWorkItem) toWorkItem() WorkItem {
	f := w.Fields
	return WorkItem{
		ID:                 w.ID,
		Rev:                w.Rev,
		Type:               WorkItemType(fieldString(f, FieldType)),
		Title:              fieldString(f, FieldTitle),
		State:              fieldString(f, FieldState),
		AssignedTo:         fieldIdentity(f, FieldAssignedTo),
		Priority:           fieldInt(f, FieldPriority),
		Description:        fieldString(f, FieldDescription),
		AcceptanceCriteria: fieldString(f, FieldAcceptanceCriteria),
		Tags:               SplitTags(fieldString(f, FieldTags)),
		AreaPath:           fieldString(f, FieldAreaPath),
		IterationPath:      fieldString(f, FieldIterationPath),
		CreatedBy:          fieldIdentity(f, FieldCreatedBy),
		CreatedDate:        fieldTime(f, FieldCreatedDate),
		ChangedDate:        fieldTime(f, FieldChangedDate),
		ParentID:           fieldInt(f, FieldParent),
	}
}

func fieldString(f map[string]any, name string) string {
	s, _ := f[name].(string)
	return s
}

func fieldInt(f map[string]any, name string) int {
	switch v := f[name].(type) {
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	}
	return 0
}

func fieldTime(f map[string]any, name string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, fieldString(f, name))
	return t
}

// fieldIdentity reads an identity field. Current API versions return an
// IdentityRef object; older ones return "Display Name <unique name>".
func fieldIdentity(f map[string]any, name string) Identity {
	switch v := f[name].(type) {
	case map[string]any:
		id := Identity{}
		id.DisplayName, _ = v["displayName"].(string)
		id.UniqueName, _ = v["uniqueName"].(string)
		id.ID, _ = v["id"].(string)
		return id
	case string:
		if open := strings.LastIndex(v, " <"); open > 0 && strings.HasSuffix(v, ">") {
			return Identity{DisplayName: v[:open], UniqueName: v[open+2 : len(v)-1]}
		}
		return Identity{DisplayName: v}
	}
	return Identity{}
}

func joinInts(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.Itoa(id)
	}
	return strings.Join(parts, ",")
}
