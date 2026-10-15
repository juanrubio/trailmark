// Package model defines the normalized session view shared by all
// provider adapters, the index, and the TUI.
package model

import "time"

// Provider identifies the coding tool that created a session.
type Provider string

const (
	ProviderClaudeCode Provider = "claude-code"
	ProviderCodex      Provider = "codex"
)

// PurposeOrigin records where a session's purpose label came from.
type PurposeOrigin string

const (
	OriginNativeTitle    PurposeOrigin = "native_title"
	OriginRequestExcerpt PurposeOrigin = "request_excerpt"
	OriginUnavailable    PurposeOrigin = "unavailable"
)

// Locator points back to one record in a read-only source log.
type Locator struct {
	SourcePath string
	Line       int // 1-based line number in the source file
	RecordID   string
}

// Message is one normalized conversation or tool record.
type Message struct {
	Seq       int
	Role      string // user, assistant, tool, system
	Kind      string // message, tool_call, tool_output
	Text      string
	Timestamp *time.Time
	Locator   Locator
}

// Session is the normalized view of one native session.
type Session struct {
	Provider       Provider
	ID             string // native session ID; may be empty if the log lacks one
	Title          string // native title, if any
	Purpose        string
	PurposeOrigin  PurposeOrigin
	CWD            string // original working directory recorded in the log
	SourcePath     string // canonical path of the source log
	Store          string // canonical directory identifying the source store
	ParentID       string // explicit parent session ID, if supplied by the provider
	ForkedFromID   string // explicit fork source session ID, if supplied
	Sidechain      bool   // provider marked the session as a sidechain/subagent run
	StartedAt      *time.Time
	LastActivityAt *time.Time
	Messages       []Message
	Partial        bool // log contained malformed or truncated records
	ParseErrors    int
}

// Capability records the validated native continuation contract for a
// provider. Templates use {id} and {cwd} placeholders and are only
// prepared or copied by Trailmark; they are never executed by the MVP.
type Capability struct {
	Provider       Provider
	ResumeTemplate string
	ForkTemplate   string // empty when ForkSupported is false
	ForkSupported  bool
	CleanTemplate  string
	Notes          string
}
