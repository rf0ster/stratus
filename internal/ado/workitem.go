// Package ado is a small Azure DevOps REST client that covers the work item
// endpoints stratus needs: WIQL queries, batch reads, JSON-patch updates,
// comments, work item type states and team members.
package ado

import (
	"slices"
	"strings"
	"time"
)

// WorkItemType is the name of an Azure DevOps work item type.
type WorkItemType string

// Common work item types across the built-in and custom processes.
const (
	Epic        WorkItemType = "Epic"
	Initiative  WorkItemType = "Initiative"
	Feature     WorkItemType = "Feature"
	Requirement WorkItemType = "Requirement"
	UserStory   WorkItemType = "User Story"
	Task        WorkItemType = "Task"
	Bug         WorkItemType = "Bug"
)

// Reference names of the fields stratus reads and writes.
const (
	FieldID                 = "System.Id"
	FieldType               = "System.WorkItemType"
	FieldTitle              = "System.Title"
	FieldState              = "System.State"
	FieldAssignedTo         = "System.AssignedTo"
	FieldPriority           = "Microsoft.VSTS.Common.Priority"
	FieldDescription        = "System.Description"
	FieldAcceptanceCriteria = "Microsoft.VSTS.Common.AcceptanceCriteria"
	FieldTags               = "System.Tags"
	FieldAreaPath           = "System.AreaPath"
	FieldIterationPath      = "System.IterationPath"
	FieldCreatedBy          = "System.CreatedBy"
	FieldCreatedDate        = "System.CreatedDate"
	FieldChangedDate        = "System.ChangedDate"
	FieldParent             = "System.Parent"
)

// itemFields is the field list requested when reading work items.
var itemFields = []string{
	FieldID,
	FieldType,
	FieldTitle,
	FieldState,
	FieldAssignedTo,
	FieldPriority,
	FieldDescription,
	FieldAcceptanceCriteria,
	FieldTags,
	FieldAreaPath,
	FieldIterationPath,
	FieldCreatedBy,
	FieldCreatedDate,
	FieldChangedDate,
	FieldParent,
}

// Identity is a reference to a user (an IdentityRef in the REST API).
type Identity struct {
	DisplayName string `json:"displayName"`
	UniqueName  string `json:"uniqueName,omitempty"`
	ID          string `json:"id,omitempty"`
}

// IsZero reports whether the identity is empty (for example, unassigned).
func (i Identity) IsZero() bool { return i.DisplayName == "" && i.UniqueName == "" }

// Name returns the best human-readable name for the identity.
func (i Identity) Name() string {
	if i.DisplayName != "" {
		return i.DisplayName
	}
	return i.UniqueName
}

// Ref returns the value to send when assigning a work item to this identity.
// The unique name (usually an email) is unambiguous, so it is preferred.
func (i Identity) Ref() string {
	if i.UniqueName != "" {
		return i.UniqueName
	}
	return i.DisplayName
}

// WorkItem is a backlog item. Description and AcceptanceCriteria hold the
// raw HTML stored by Azure DevOps; use HTMLToText to display them.
type WorkItem struct {
	ID                 int
	Rev                int
	Type               WorkItemType
	Title              string
	State              string
	AssignedTo         Identity
	Priority           int
	Description        string
	AcceptanceCriteria string
	Tags               []string
	AreaPath           string
	IterationPath      string
	CreatedBy          Identity
	CreatedDate        time.Time
	ChangedDate        time.Time
	ParentID           int
}

// Comment is a work item discussion comment. Text is HTML.
type Comment struct {
	ID          int       `json:"id"`
	Text        string    `json:"text"`
	CreatedBy   Identity  `json:"createdBy"`
	CreatedDate time.Time `json:"createdDate"`
}

// State is a workflow state of a work item type.
type State struct {
	Name     string `json:"name"`
	Color    string `json:"color"`
	Category string `json:"category"`
}

// Change sets a single field on a work item. Value should be a string or a
// number; an empty string clears the field.
type Change struct {
	Field string
	Value any
}

// SplitTags parses a tag list separated by semicolons or commas. Azure DevOps
// tags can't contain either character, so both are safe separators.
func SplitTags(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == ',' })
	tags := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if slices.ContainsFunc(tags, func(t string) bool { return strings.EqualFold(t, p) }) {
			continue
		}
		tags = append(tags, p)
	}
	return tags
}

// JoinTags formats tags the way Azure DevOps stores them.
func JoinTags(tags []string) string { return strings.Join(tags, "; ") }
