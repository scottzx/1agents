// Command release-notes generates a self-contained HTML page introducing the
// features added in a release. It reads merged-PR / commit subjects from
// `git log` over a version range, groups them by conventional-commit type, and
// renders an inline-styled HTML document that can be opened directly.
//
// Implements GitHub issue #145 (re-scoped: the original asked for an
// auto-generated product *video*; per maintainer decision the video is deferred
// and this generates an HTML feature-introduction page instead).
//
// Usage:
//
//	go run ./cmd/release-notes [flags]
//	release-notes -from v20260623-1 -to HEAD -o release.html
//
// With no -from, the previous tag (relative to -to) is used; if there is no
// previous tag the whole history is included. PR links require a -repo
// (auto-detected from `git remote` when omitted); detection degrades gracefully
// when offline since everything is derived from local git only.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

func main() {
	var (
		from = flag.String("from", "", "lower bound ref (tag/commit). Default: previous tag relative to -to")
		to   = flag.String("to", "HEAD", "upper bound ref")
		repo = flag.String("repo", "", "owner/name for PR links. Default: detected from git remote")
		ver  = flag.String("version", "", "display version. Default: -to if it is a tag, else `git describe`")
		out  = flag.String("o", "", "output HTML file. Default: stdout")
	)
	flag.Parse()

	g := gitRunner{}
	if err := run(g, *from, *to, *repo, *ver, *out); err != nil {
		fmt.Fprintln(os.Stderr, "release-notes:", err)
		os.Exit(1)
	}
}

// git abstracts the few git commands we need so the range/version resolution
// logic stays testable.
type git interface {
	run(args ...string) (string, error)
}

type gitRunner struct{}

func (gitRunner) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

func run(g git, from, to, repo, ver, out string) error {
	if from == "" {
		from = previousTag(g, to)
	}
	if repo == "" {
		repo = detectRepo(g)
	}
	if ver == "" {
		ver = displayVersion(g, to)
	}

	subjects, err := collectSubjects(g, from, to)
	if err != nil {
		return err
	}
	features := ParseLog(subjects)
	groups := GroupFeatures(features)

	data := PageData{
		Version:    ver,
		PrevRef:    from,
		CommitTo:   to,
		Repo:       repo,
		Generated:  time.Now().UTC().Format("2006-01-02 15:04 UTC"),
		Groups:     groups,
		TotalCount: len(features),
	}
	html, err := Render(data)
	if err != nil {
		return err
	}

	if out == "" {
		_, err = os.Stdout.WriteString(html)
		return err
	}
	if err := os.WriteFile(out, []byte(html), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "release-notes: wrote %d features to %s\n", len(features), out)
	return nil
}

// collectSubjects returns the commit subject lines in (from, to]. When from is
// empty the full history up to `to` is used.
func collectSubjects(g git, from, to string) ([]string, error) {
	rng := to
	if from != "" {
		rng = from + ".." + to
	}
	out, err := g.run("log", "--no-merges", "--pretty=format:%s", rng)
	if err != nil {
		return nil, fmt.Errorf("git log %s: %w", rng, err)
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// previousTag returns the most recent tag reachable before `to`, or "" if none.
func previousTag(g git, to string) string {
	t, err := g.run("describe", "--tags", "--abbrev=0", to+"^")
	if err != nil {
		// `to` may itself not be a tag; try the tag before HEAD's nearest tag.
		t, err = g.run("describe", "--tags", "--abbrev=0", to)
		if err != nil {
			return ""
		}
	}
	return t
}

// displayVersion picks a human version label: the tag at `to` if there is one,
// otherwise `git describe`.
func displayVersion(g git, to string) string {
	if t, err := g.run("describe", "--tags", "--exact-match", to); err == nil && t != "" {
		return t
	}
	if d, err := g.run("describe", "--tags", "--always", to); err == nil && d != "" {
		return d
	}
	return to
}

var remoteRE = regexp.MustCompile(`github\.com[:/]([^/]+/[^/.]+)(\.git)?`)

// detectRepo parses `owner/name` from the origin remote URL. Returns "" when no
// GitHub remote is configured (offline / non-GitHub), in which case PR numbers
// render as plain text.
func detectRepo(g git) string {
	url, err := g.run("config", "--get", "remote.origin.url")
	if err != nil || url == "" {
		return ""
	}
	if m := remoteRE.FindStringSubmatch(url); m != nil {
		return strings.TrimSuffix(m[1], ".git")
	}
	return ""
}
