// Package kwiki is the knowledge-base storage substrate for the Inbox context
// engine (#191), modelled on Karpathy's kwiki "compile knowledge once, then
// query" idea: raw → wiki → output (see docs/features/inbox-context-engine
// design §3.3).
//
//	raw/    收口原文        — captured external context, one file per Inbox item
//	wiki/   分类后知识      — compiled knowledge pages (concepts/summaries/tags)
//	output/ 转出的需求/报告  — articles/reports generated out of the wiki
//
// The package is a self-contained file-tree library: it owns nothing but a
// directory layout under a single Root. It deliberately does NOT depend on the
// (still-unbuilt) Inbox table (#60) or any UI — an "Inbox item" here is a plain
// Go struct passed in by the caller. There is no vector store; navigation is
// frontmatter tags + a generated index.md (简单优先).
//
// Operations:
//
//	Ingest    compile one Inbox item into a wiki page (concepts/summary/tags),
//	          append an .ingested.json provenance record, regenerate index.md.
//	FileBack  append a conversation insight onto an existing wiki page.
//	Lint      report broken wiki links and orphan pages.
package kwiki

import (
	"fmt"
	"os"
	"path/filepath"
)

// Layer directory names under Root.
const (
	rawDir    = "raw"
	wikiDir   = "wiki"
	outputDir = "output"

	ingestedLog = ".ingested.json" // provenance log, lives at Root
	indexFile   = "index.md"       // generated nav, lives in wiki/
)

// Store is a kwiki knowledge base rooted at a single directory. The zero value
// is not usable; construct with Open.
type Store struct {
	Root string
}

// Open returns a Store rooted at root, creating the raw/wiki/output layout if
// it does not yet exist.
func Open(root string) (*Store, error) {
	if root == "" {
		return nil, fmt.Errorf("kwiki: empty root")
	}
	s := &Store{Root: root}
	for _, d := range []string{rawDir, wikiDir, outputDir} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			return nil, fmt.Errorf("kwiki: create %s: %w", d, err)
		}
	}
	return s, nil
}

func (s *Store) rawPath(name string) string  { return filepath.Join(s.Root, rawDir, name) }
func (s *Store) wikiPath(name string) string { return filepath.Join(s.Root, wikiDir, name) }
func (s *Store) indexPath() string           { return filepath.Join(s.Root, wikiDir, indexFile) }
func (s *Store) ingestedLogPath() string     { return filepath.Join(s.Root, ingestedLog) }
