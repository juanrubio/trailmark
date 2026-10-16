// Package memory discovers and parses read-only provider memory
// stores (Claude Code and Codex) into a shared catalog. It never
// writes provider files and has no terminal-rendering dependencies.
package memory

import (
	"time"

	"github.com/juanrubio/trailmark/internal/model"
)

// Category classifies a memory document or entry by role.
type Category string

const (
	CategoryNote         Category = "note"
	CategoryIndex        Category = "index"
	CategoryConsolidated Category = "consolidated"
	CategoryOverview     Category = "overview"
	CategorySupporting   Category = "supporting"
	CategoryRaw          Category = "raw"
)

// DefaultCategories are the categories shown by default browsing.
func DefaultCategories() []Category { return []Category{CategoryNote, CategoryConsolidated} }

// DefaultMaxFileBytes caps one source file when Options leaves it unset.
const DefaultMaxFileBytes int64 = 2 << 20

// Root is one allowed memory location. For Claude it is a projects
// directory (enumerating */memory) or a direct memory directory. For
// Codex it is a memories directory or a Codex home containing one.
type Root struct {
	Provider model.Provider
	Path     string
}

// Document is one source file inside an allowed root.
type Document struct {
	ID, Path, Root  string
	Provider        model.Provider
	Category        Category
	Title           string
	ModTime         time.Time
	Status          string // ok, unreadable, truncated, unsupported, outside-root
	StatusDetail    string
	ProjectCWD      string
	ProjectEvidence string // session-association, rollout-header, directory-name, global, unknown
}

// Reference is one link or identifier recorded inside an entry.
type Reference struct {
	Raw, Kind string // Kind: note, session, url, path
	EntryID   string
	SessionID string
	Status    string // resolved, unresolved, external
}

// Entry is one browsable memory: a whole note or one section of a
// larger document. StartLine/EndLine are 1-based in the source file.
type Entry struct {
	ID, DocumentID, Path string
	Provider             model.Provider
	Category             Category
	Title, Description   string
	Type                 string
	HeadingPath          []string
	StartLine, EndLine   int
	TextLine             int    // 1-based source line where Text begins, as recorded by the parser
	Text                 string // body markdown, no frontmatter
	RawText              string // exact source slice incl headings/frontmatter
	Meta                 map[string]string
	OriginSessionID      string
	ThreadID             string
	RecordedAt           *time.Time
	ModTime              time.Time
	ProjectCWD           string
	ProjectEvidence      string
	References           []Reference
}

// SourceStatus reports discovery outcome for one root.
type SourceStatus struct {
	Provider  model.Provider
	Root      string
	State     string // ok, missing, unreadable, empty, excluded
	Detail    string
	Documents int
	Entries   int
}

// Catalog is the in-memory result of one Load.
type Catalog struct {
	Documents []Document
	Entries   []Entry
	Sources   []SourceStatus
}

// Options controls one Load.
type Options struct {
	Roots    []Root
	Excluded func(path string) bool
	// ClaudeProjectByDir maps a canonical Claude project directory
	// (the parent of a memory directory) to its recorded cwd.
	ClaudeProjectByDir map[string]string
	MaxFileBytes       int64 // 0 => 2MiB
}

// Filter narrows entries for browsing and search.
type Filter struct {
	Provider   model.Provider // "" = all
	Project    string         // "" = all, "unknown" = empty cwd, else exact cwd plus global entries
	Categories []Category     // empty = all
	Text       string         // case-insensitive title/description/heading contains
	// ProjectRelated widens an exact Project match to directories
	// nested either way (the session list's Current project
	// semantics, where a session recorded in a repo subdirectory
	// belongs to the repo root's scope).
	ProjectRelated bool
}

// Match groups search hits by entry.
type Match struct {
	Entry    Entry
	Count    int
	Lines    []int // source lines
	Snippets []string
}
