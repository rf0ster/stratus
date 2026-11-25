// Package mockserver is an in-memory fake of the Azure DevOps work item REST
// API. It serves the same endpoints and JSON shapes the real service does, so
// the real client can be pointed at it. All changes live in memory and are
// lost when the server stops.
package mockserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"mime"
	"net"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultOrg is the organization segment used in mock server URLs.
	// Any organization name is accepted.
	DefaultOrg = "mock"
	// DefaultProject is the name of the mock project.
	DefaultProject = "Stratus"

	maxBatchSize = 200
	maxWIQLTop   = 20000
)

// Options configures a Server.
type Options struct {
	// Project overrides the project name (default "Stratus").
	Project string
	// Latency delays every response, to simulate a slow network.
	Latency time.Duration
	// Logger, if set, logs one line per request.
	Logger *log.Logger
}

// Server is a fake Azure DevOps server. It implements http.Handler.
type Server struct {
	opts    Options
	project string
	handler http.Handler

	mu            sync.Mutex
	me            identity
	team          []identity
	states        map[string][]state // keyed by lower-case work item type
	items         map[int]*item
	nextCommentID int
}

type identity struct {
	DisplayName string `json:"displayName"`
	UniqueName  string `json:"uniqueName"`
	ID          string `json:"id"`
}

// matches reports whether s names this identity, the way Azure DevOps
// resolves identity values: display name, unique name, or "Name <unique>".
func (i identity) matches(s string) bool {
	s = strings.TrimSpace(s)
	return strings.EqualFold(s, i.DisplayName) ||
		strings.EqualFold(s, i.UniqueName) ||
		strings.EqualFold(s, i.DisplayName+" <"+i.UniqueName+">")
}

type state struct {
	Name     string `json:"name"`
	Color    string `json:"color"`
	Category string `json:"category"`
}

type item struct {
	id       int
	rev      int
	fields   map[string]any // values are string, float64 or identity
	comments []*comment
}

type comment struct {
	WorkItemID   int       `json:"workItemId"`
	ID           int       `json:"id"`
	Version      int       `json:"version"`
	Text         string    `json:"text"`
	CreatedBy    identity  `json:"createdBy"`
	CreatedDate  time.Time `json:"createdDate"`
	ModifiedBy   identity  `json:"modifiedBy"`
	ModifiedDate time.Time `json:"modifiedDate"`
}

// Field reference names known to the mock server.
const (
	fID                 = "System.Id"
	fType               = "System.WorkItemType"
	fTitle              = "System.Title"
	fState              = "System.State"
	fAssignedTo         = "System.AssignedTo"
	fPriority           = "Microsoft.VSTS.Common.Priority"
	fDescription        = "System.Description"
	fAcceptanceCriteria = "Microsoft.VSTS.Common.AcceptanceCriteria"
	fTags               = "System.Tags"
	fAreaPath           = "System.AreaPath"
	fIterationPath      = "System.IterationPath"
	fCreatedBy          = "System.CreatedBy"
	fCreatedDate        = "System.CreatedDate"
	fChangedBy          = "System.ChangedBy"
	fChangedDate        = "System.ChangedDate"
	fParent             = "System.Parent"
	fTeamProject        = "System.TeamProject"
)

var knownFields = []string{
	fID, fType, fTitle, fState, fAssignedTo, fPriority, fDescription,
	fAcceptanceCriteria, fTags, fAreaPath, fIterationPath, fCreatedBy,
	fCreatedDate, fChangedBy, fChangedDate, fParent, fTeamProject,
}

var readOnlyFields = []string{fID, fType, fCreatedBy, fCreatedDate, fChangedBy, fChangedDate, fTeamProject}

var identityFields = []string{fAssignedTo, fCreatedBy, fChangedBy}

// canonicalField resolves a field reference name case-insensitively.
func canonicalField(name string) (string, bool) {
	for _, f := range knownFields {
		if strings.EqualFold(f, name) {
			return f, true
		}
	}
	return "", false
}

// New returns a server seeded with a sample backlog.
func New(opts Options) *Server {
	s := &Server{opts: opts, project: opts.Project}
	if s.project == "" {
		s.project = DefaultProject
	}
	s.seed(time.Now())

	mux := http.NewServeMux()
	mux.HandleFunc("POST /{org}/{project}/_apis/wit/wiql", s.withProject(s.handleWIQL))
	mux.HandleFunc("GET /{org}/{project}/_apis/wit/workitems", s.withProject(s.handleGetWorkItems))
	mux.HandleFunc("GET /{org}/{project}/_apis/wit/workitems/{id}", s.withProject(s.handleGetWorkItem))
	mux.HandleFunc("PATCH /{org}/{project}/_apis/wit/workitems/{id}", s.withProject(s.handleUpdateWorkItem))
	mux.HandleFunc("GET /{org}/{project}/_apis/wit/workitems/{id}/comments", s.withProject(s.handleGetComments))
	mux.HandleFunc("POST /{org}/{project}/_apis/wit/workitems/{id}/comments", s.withProject(s.handleAddComment))
	mux.HandleFunc("GET /{org}/{project}/_apis/wit/workitemtypes/{type}/states", s.withProject(s.handleGetStates))
	mux.HandleFunc("GET /{org}/_apis/projects/{project}/teams/{team}/members", s.withProject(s.handleGetTeamMembers))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("The mock server doesn't implement %s %s.", r.Method, r.URL.Path))
	})
	s.handler = mux
	return s
}

// Project returns the mock project name.
func (s *Server) Project() string { return s.project }

// Me returns the display name of the user the mock server treats as signed in.
func (s *Server) Me() string { return s.me.DisplayName }

// Start serves on addr (for example "127.0.0.1:0") in the background. It
// returns the organization URL to give the client and a function that stops
// the server.
func (s *Server) Start(addr string) (orgURL string, stop func() error, err error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", nil, err
	}
	srv := &http.Server{Handler: s, ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln) //nolint:errcheck // Serve always returns an error after Shutdown.

	stop = func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	}
	// A wildcard address (":8080", "0.0.0.0:8080") isn't something a client
	// can dial, so point the URL at loopback instead.
	host := ln.Addr().String()
	if tcp, ok := ln.Addr().(*net.TCPAddr); ok && tcp.IP.IsUnspecified() {
		host = net.JoinHostPort("127.0.0.1", strconv.Itoa(tcp.Port))
	}
	return "http://" + host + "/" + DefaultOrg, stop, nil
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

	if s.opts.Latency > 0 {
		select {
		case <-time.After(s.opts.Latency):
		case <-r.Context().Done():
			return
		}
	}

	// Azure DevOps routes are case-insensitive ("workItems" and "workitems"
	// both work), so normalize everything after /_apis/.
	if i := strings.Index(strings.ToLower(r.URL.Path), "/_apis/"); i >= 0 {
		r.URL.Path = r.URL.Path[:i] + strings.ToLower(r.URL.Path[i:])
		r.URL.RawPath = ""
	}

	s.handler.ServeHTTP(rec, r)

	if s.opts.Logger != nil {
		s.opts.Logger.Printf("%s %s -> %d (%s)", r.Method, r.URL.RequestURI(), rec.status, time.Since(start).Round(time.Millisecond))
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// withProject checks the project path segment and the api-version parameter
// before calling h.
func (s *Server) withProject(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := r.PathValue("project"); !strings.EqualFold(p, s.project) {
			writeError(w, http.StatusNotFound, fmt.Sprintf("TF200016: The following project does not exist: %s. (The mock project is %q.)", p, s.project))
			return
		}
		if r.URL.Query().Get("api-version") == "" {
			writeError(w, http.StatusBadRequest, "No api-version was supplied for the request. An api-version must be supplied.")
			return
		}
		h(w, r)
	}
}

func (s *Server) handleWIQL(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Query) == "" {
		writeError(w, http.StatusBadRequest, "The request body must be a JSON object with a 'query' property.")
		return
	}
	q, err := parseWIQL(body.Query, canonicalField)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	top := maxWIQLTop
	if v := r.URL.Query().Get("$top"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "$top must be a positive integer.")
			return
		}
		top = min(n, maxWIQLTop)
	}

	s.mu.Lock()
	ids := q.run(&evalContext{me: s.me, project: s.project}, s.sortedItems())
	s.mu.Unlock()

	if len(ids) > top {
		ids = ids[:top]
	}
	refs := make([]map[string]any, len(ids))
	for i, id := range ids {
		refs[i] = map[string]any{"id": id, "url": s.itemURL(r, id)}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"queryType":       "flat",
		"queryResultType": "workItem",
		"asOf":            time.Now().UTC(),
		"workItems":       refs,
	})
}

func (s *Server) handleGetWorkItems(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	var ids []int
	for _, part := range strings.Split(query.Get("ids"), ",") {
		if part = strings.TrimSpace(part); part == "" {
			continue
		}
		id, err := strconv.Atoi(part)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid work item id %q.", part))
			return
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		writeError(w, http.StatusBadRequest, "The ids parameter is required.")
		return
	}
	if len(ids) > maxBatchSize {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("VS403474: You requested %d work items, which exceeds the limit of %d.", len(ids), maxBatchSize))
		return
	}
	fields := splitFields(query.Get("fields"))
	omitMissing := strings.EqualFold(query.Get("errorPolicy"), "omit")

	s.mu.Lock()
	defer s.mu.Unlock()
	values := make([]any, 0, len(ids))
	for _, id := range ids {
		it, ok := s.items[id]
		switch {
		case ok:
			values = append(values, s.itemJSON(r, it, fields))
		case omitMissing:
			values = append(values, nil)
		default:
			writeError(w, http.StatusNotFound, missingItemMessage(id))
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(values), "value": values})
}

func (s *Server) handleGetWorkItem(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.lookup(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.itemJSON(r, it, splitFields(r.URL.Query().Get("fields"))))
}

type patchOp struct {
	Op    string          `json:"op"`
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value"`
}

func (s *Server) handleUpdateWorkItem(w http.ResponseWriter, r *http.Request) {
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json-patch+json" {
		writeError(w, http.StatusUnsupportedMediaType, fmt.Sprintf(
			"The request indicated a Content-Type of %q for method type PATCH which is not supported. Valid content types for this method are: application/json-patch+json.",
			r.Header.Get("Content-Type")))
		return
	}
	var ops []patchOp
	if err := json.NewDecoder(r.Body).Decode(&ops); err != nil {
		writeError(w, http.StatusBadRequest, "The request body must be a JSON patch document (an array of operations).")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.lookup(w, r)
	if !ok {
		return
	}

	// Apply to a copy so a failing operation leaves the item untouched.
	fields := make(map[string]any, len(it.fields))
	for k, v := range it.fields {
		fields[k] = v
	}
	changed := false
	for _, op := range ops {
		status, err := s.applyOp(it, fields, op)
		if err != nil {
			writeError(w, status, err.Error())
			return
		}
		changed = changed || !strings.EqualFold(op.Op, "test")
	}

	if changed {
		it.fields = fields
		it.rev++
		it.fields[fChangedBy] = s.me
		it.fields[fChangedDate] = formatTime(time.Now())
	}
	writeJSON(w, http.StatusOK, s.itemJSON(r, it, nil))
}

// applyOp validates one patch operation and applies it to fields. It returns
// the HTTP status to use when the operation is rejected.
func (s *Server) applyOp(it *item, fields map[string]any, op patchOp) (int, error) {
	switch strings.ToLower(op.Op) {
	case "test":
		if !strings.EqualFold(op.Path, "/rev") {
			return http.StatusBadRequest, fmt.Errorf("the mock server only supports test operations on /rev")
		}
		var rev int
		if err := json.Unmarshal(op.Value, &rev); err != nil {
			return http.StatusBadRequest, fmt.Errorf("the /rev test value must be a number")
		}
		if rev != it.rev {
			return http.StatusPreconditionFailed, fmt.Errorf(
				"TF26071: This work item has been changed by someone else since you opened it (rev %d, current rev %d). Refresh it and try again.", rev, it.rev)
		}
		return 0, nil
	case "add", "replace", "remove":
	default:
		return http.StatusBadRequest, fmt.Errorf("unsupported patch operation %q", op.Op)
	}

	name, ok := strings.CutPrefix(op.Path, "/fields/")
	if !ok {
		return http.StatusBadRequest, fmt.Errorf("the mock server only supports /fields/* paths, not %q", op.Path)
	}
	field, ok := canonicalField(name)
	if !ok {
		return http.StatusBadRequest, fmt.Errorf("TF51535: Cannot find field %s.", name)
	}
	if slices.Contains(readOnlyFields, field) {
		return http.StatusBadRequest, fmt.Errorf("TF401326: Invalid field status 'ReadOnly' for field '%s'.", field)
	}

	var raw any
	if strings.ToLower(op.Op) != "remove" && len(op.Value) > 0 {
		if err := json.Unmarshal(op.Value, &raw); err != nil {
			return http.StatusBadRequest, fmt.Errorf("invalid value for %s", field)
		}
	}
	if raw == nil || raw == "" {
		if field == fTitle || field == fState {
			return http.StatusBadRequest, fmt.Errorf("TF401320: Rule Error for field %s. Error code: Required, InvalidEmpty.", field)
		}
		delete(fields, field)
		return 0, nil
	}

	switch field {
	case fAssignedTo:
		name, _ := raw.(string)
		member, ok := s.findMember(name)
		if !ok {
			return http.StatusBadRequest, fmt.Errorf("The identity value '%v' for field 'Assigned To' is an unknown identity.", raw)
		}
		fields[field] = member
	case fPriority, fParent:
		n, ok := toNumber(raw)
		if !ok || (field == fPriority && (n < 1 || n > 4)) {
			return http.StatusBadRequest, fmt.Errorf("TF401320: Rule Error for field %s. Error code: InvalidListValue. The value '%v' is not allowed.", field, raw)
		}
		if field == fParent {
			if _, exists := s.items[int(n)]; !exists {
				return http.StatusBadRequest, fmt.Errorf("parent work item %v does not exist", raw)
			}
		}
		fields[field] = n
	case fState:
		name, _ := raw.(string)
		st, ok := s.findState(textOf(fields[fType]), name)
		if !ok {
			return http.StatusBadRequest, fmt.Errorf(
				"TF401320: Rule Error for field State. Error code: InvalidListValue. '%v' is not a valid state for a %s.", raw, textOf(fields[fType]))
		}
		fields[field] = st.Name
	case fTags:
		fields[field] = normalizeTags(textOf(raw))
	default:
		str, ok := raw.(string)
		if !ok {
			return http.StatusBadRequest, fmt.Errorf("the value for %s must be a string", field)
		}
		if field == fTitle && len([]rune(str)) > 255 {
			return http.StatusBadRequest, fmt.Errorf("TF401320: Rule Error for field Title. Error code: MaxLength (255).")
		}
		fields[field] = str
	}
	return 0, nil
}

func (s *Server) handleGetComments(w http.ResponseWriter, r *http.Request) {
	if !requirePreview(w, r) {
		return
	}
	query := r.URL.Query()
	top := 200
	if v := query.Get("$top"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			top = min(n, 200)
		}
	}
	offset, _ := strconv.Atoi(query.Get("continuationToken"))

	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.lookup(w, r)
	if !ok {
		return
	}
	comments := slices.Clone(it.comments)
	sort.SliceStable(comments, func(i, j int) bool {
		if strings.EqualFold(query.Get("order"), "desc") {
			return comments[i].CreatedDate.After(comments[j].CreatedDate)
		}
		return comments[i].CreatedDate.Before(comments[j].CreatedDate)
	})

	page := comments[min(max(offset, 0), len(comments)):]
	resp := map[string]any{"totalCount": len(comments)}
	if len(page) > top {
		page = page[:top]
		resp["continuationToken"] = strconv.Itoa(offset + top)
	}
	resp["count"] = len(page)
	resp["comments"] = page
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAddComment(w http.ResponseWriter, r *http.Request) {
	if !requirePreview(w, r) {
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Text) == "" {
		writeError(w, http.StatusBadRequest, "The comment text must not be empty.")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.lookup(w, r)
	if !ok {
		return
	}
	now := time.Now().UTC()
	s.nextCommentID++
	c := &comment{
		WorkItemID: it.id, ID: s.nextCommentID, Version: 1, Text: body.Text,
		CreatedBy: s.me, CreatedDate: now, ModifiedBy: s.me, ModifiedDate: now,
	}
	it.comments = append(it.comments, c)
	// Comments are part of the work item history, so they add a revision.
	it.rev++
	it.fields[fChangedBy] = s.me
	it.fields[fChangedDate] = formatTime(now)
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleGetStates(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	states, ok := s.states[strings.ToLower(r.PathValue("type"))]
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("VS402323: Work item type %s does not exist.", r.PathValue("type")))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(states), "value": states})
}

func (s *Server) handleGetTeamMembers(w http.ResponseWriter, r *http.Request) {
	team := r.PathValue("team")
	if !strings.EqualFold(team, s.project+" Team") {
		writeError(w, http.StatusNotFound, fmt.Sprintf("The team with id %s does not exist. (The mock team is %q.)", team, s.project+" Team"))
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	members := make([]map[string]any, len(s.team))
	for i, m := range s.team {
		members[i] = map[string]any{"identity": m, "isTeamAdmin": i == 0}
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(members), "value": members})
}

// lookup finds the item named by the {id} path segment, writing a 404 if it
// doesn't exist. The caller must hold s.mu.
func (s *Server) lookup(w http.ResponseWriter, r *http.Request) (*item, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid work item id %q.", r.PathValue("id")))
		return nil, false
	}
	it, ok := s.items[id]
	if !ok {
		writeError(w, http.StatusNotFound, missingItemMessage(id))
		return nil, false
	}
	return it, true
}

func (s *Server) sortedItems() []*item {
	items := make([]*item, 0, len(s.items))
	for _, it := range s.items {
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].id < items[j].id })
	return items
}

func (s *Server) findMember(name string) (identity, bool) {
	for _, m := range s.team {
		if m.matches(name) {
			return m, true
		}
	}
	return identity{}, false
}

func (s *Server) findState(workItemType, name string) (state, bool) {
	for _, st := range s.states[strings.ToLower(workItemType)] {
		if strings.EqualFold(st.Name, name) {
			return st, true
		}
	}
	return state{}, false
}

func (s *Server) itemURL(r *http.Request, id int) string {
	return fmt.Sprintf("http://%s/%s/%s/_apis/wit/workItems/%d", r.Host, r.PathValue("org"), s.project, id)
}

// itemJSON renders an item, limited to the requested fields (all if empty).
// The caller must hold s.mu.
func (s *Server) itemJSON(r *http.Request, it *item, fields []string) map[string]any {
	out := make(map[string]any, len(it.fields))
	for name, v := range it.fields {
		if len(fields) == 0 || slices.ContainsFunc(fields, func(f string) bool { return strings.EqualFold(f, name) }) {
			out[name] = v
		}
	}
	return map[string]any{"id": it.id, "rev": it.rev, "fields": out, "url": s.itemURL(r, it.id)}
}

func requirePreview(w http.ResponseWriter, r *http.Request) bool {
	if v := r.URL.Query().Get("api-version"); !strings.Contains(v, "-preview") {
		writeError(w, http.StatusBadRequest, fmt.Sprintf(
			"The requested version %q of the resource is under preview. The -preview flag must be supplied in the api-version for such requests.", v))
		return false
	}
	return true
}

func missingItemMessage(id int) string {
	return fmt.Sprintf("TF401232: Work item %d does not exist, or you do not have permissions to read it.", id)
}

func splitFields(s string) []string {
	var fields []string
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			fields = append(fields, f)
		}
	}
	return fields
}

func normalizeTags(s string) string {
	var tags []string
	for _, t := range strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == ',' }) {
		t = strings.TrimSpace(t)
		if t != "" && !slices.ContainsFunc(tags, func(x string) bool { return strings.EqualFold(x, t) }) {
			tags = append(tags, t)
		}
	}
	sort.Slice(tags, func(i, j int) bool { return strings.ToLower(tags[i]) < strings.ToLower(tags[j]) })
	return strings.Join(tags, "; ")
}

func toNumber(v any) (float64, bool) {
	switch v := v.(type) {
	case float64:
		return v, v == float64(int(v))
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		return float64(n), err == nil
	}
	return 0, false
}

func formatTime(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) // the client has gone away if this fails
}

// writeError writes an error in the shape Azure DevOps uses.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"$id":            "1",
		"innerException": nil,
		"message":        message,
		"typeName":       "Microsoft.VisualStudio.Services.Common.VssServiceException, Microsoft.VisualStudio.Services.Common",
		"typeKey":        "VssServiceException",
		"errorCode":      0,
		"eventId":        3000,
	})
}
