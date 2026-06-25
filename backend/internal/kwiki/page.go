package kwiki

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Marshal renders the page as a YAML-frontmatter Markdown document.
func (p WikiPage) Marshal() ([]byte, error) {
	var fm bytes.Buffer
	enc := yaml.NewEncoder(&fm)
	enc.SetIndent(2)
	if err := enc.Encode(p); err != nil {
		return nil, fmt.Errorf("kwiki: encode frontmatter: %w", err)
	}
	_ = enc.Close()

	var out bytes.Buffer
	out.WriteString("---\n")
	out.Write(fm.Bytes())
	out.WriteString("---\n")
	body := strings.TrimLeft(p.Body, "\n")
	if body != "" {
		out.WriteByte('\n')
		out.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			out.WriteByte('\n')
		}
	}
	return out.Bytes(), nil
}

// parsePage parses a YAML-frontmatter Markdown document into a WikiPage. Slug is
// not set here (it comes from the filename); the caller fills it in.
func parsePage(data []byte) (WikiPage, error) {
	fm, body := splitFrontmatter(data)
	var p WikiPage
	if len(fm) > 0 {
		if err := yaml.Unmarshal(fm, &p); err != nil {
			return WikiPage{}, fmt.Errorf("kwiki: parse frontmatter: %w", err)
		}
	}
	p.Body = string(body)
	return p, nil
}

// loadPage reads and parses the wiki page with the given slug (no .md suffix).
func (s *Store) loadPage(slug string) (WikiPage, error) {
	data, err := os.ReadFile(s.wikiPath(slug + ".md"))
	if err != nil {
		return WikiPage{}, err
	}
	p, err := parsePage(data)
	if err != nil {
		return WikiPage{}, fmt.Errorf("kwiki: %s: %w", slug, err)
	}
	p.Slug = slug
	return p, nil
}

// splitFrontmatter splits a document into its raw YAML frontmatter block
// (without the `---` fences) and the trailing body. With no leading fence it
// returns (nil, doc).
func splitFrontmatter(doc []byte) (frontmatter, body []byte) {
	s := bytes.TrimPrefix(doc, []byte("\uFEFF")) // tolerate a leading BOM
	s = bytes.ReplaceAll(s, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(s, []byte("---\n")) {
		return nil, doc
	}
	rest := s[len("---\n"):]
	// Find the closing fence at the start of a line.
	end := bytes.Index(rest, []byte("\n---\n"))
	if end < 0 {
		if bytes.HasSuffix(rest, []byte("\n---")) {
			return rest[:len(rest)-len("\n---")], nil
		}
		return nil, doc // malformed: no closing fence
	}
	fm := rest[:end+1] // keep trailing newline of last fm line
	bd := rest[end+len("\n---\n"):]
	return fm, bytes.TrimLeft(bd, "\n")
}
