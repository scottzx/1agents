package projectitems

import (
	"encoding/json"
	"flag"
	"fmt"
	"strings"
)

const featureCatalogCLIUsage = `usage: 1agents feature-catalog <verb> [flags]   (run inside a project directory)

  list
  create   --kind module|feature --title T [--parent ID] [--description D]
           [--target-milestone ID] [--position N]
  update   <id> [--title T] [--description D] [--target-milestone ID]
  move     <id> [--parent ID] [--position N]
  link     <feature-id> --item ID --relation source|delivery
  unlink   <feature-id> --item ID --relation source|delivery
  batch    --json '<operations array>'
  gantt    [--json]
  export   --format markdown|json

The project is locked from ONEAGENTS_WORKSPACE_ID, then --project, then cwd.
Batch create operations may publish clientRef; parentRef, nodeRef, and featureRef
resolve those server-generated ids inside the same atomic transaction.`

// RunFeatureCatalogCLI dispatches the Bash-facing feature-catalog command.
// Every verb goes through Client and therefore the same REST/store semantics as
// Web and the MCP tools.
func RunFeatureCatalogCLI(args []string) int {
	if len(args) == 0 {
		fmt.Println(featureCatalogCLIUsage)
		return 1
	}
	switch args[0] {
	case "help", "-h", "--help":
		fmt.Println(featureCatalogCLIUsage)
		return 0
	case "list":
		return cliFeatureCatalogList(args[1:])
	case "create":
		return cliFeatureCatalogCreate(args[1:])
	case "update":
		return cliFeatureCatalogUpdate(args[1:])
	case "move":
		return cliFeatureCatalogMove(args[1:])
	case "link":
		return cliFeatureCatalogLink(args[1:], false)
	case "unlink":
		return cliFeatureCatalogLink(args[1:], true)
	case "batch":
		return cliFeatureCatalogBatch(args[1:])
	case "gantt":
		return cliFeatureCatalogGantt(args[1:])
	case "export":
		return cliFeatureCatalogExport(args[1:])
	default:
		return cliFail("unknown feature-catalog verb %q\n%s", args[0], featureCatalogCLIUsage)
	}
}

func cliFeatureCatalogList(args []string) int {
	fs := flag.NewFlagSet("feature-catalog list", flag.ContinueOnError)
	project := fs.String("project", "", "project id|name|path")
	_ = fs.Bool("json", false, "raw daemon JSON")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	c, _, code := cliClient(*project)
	if code >= 0 {
		return code
	}
	return emitRaw(c.ListFeatureCatalog())
}

func cliFeatureCatalogCreate(args []string) int {
	fs := flag.NewFlagSet("feature-catalog create", flag.ContinueOnError)
	project := fs.String("project", "", "project id|name|path")
	kind := fs.String("kind", "", "module|feature")
	title := fs.String("title", "", "node title")
	parent := fs.String("parent", "", "parent module id")
	description := fs.String("description", "", "description")
	target := fs.String("target-milestone", "", "semantic milestone id")
	position := fs.Int("position", 0, "zero-based sibling position")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *kind != "module" && *kind != "feature" {
		return cliFail("--kind must be module or feature")
	}
	if strings.TrimSpace(*title) == "" {
		return cliFail("--title is required")
	}
	c, _, code := cliClient(*project)
	if code >= 0 {
		return code
	}
	return emitRaw(c.CreateFeatureNode(map[string]any{
		"kind": *kind, "title": *title, "parentId": *parent,
		"description": *description, "targetMilestoneId": *target,
		"position": *position,
	}))
}

func cliFeatureCatalogUpdate(args []string) int {
	id, rest := splitLeadingID(args)
	fs := flag.NewFlagSet("feature-catalog update", flag.ContinueOnError)
	project := fs.String("project", "", "project id|name|path")
	title := fs.String("title", "", "node title")
	description := fs.String("description", "", "description")
	target := fs.String("target-milestone", "", "semantic milestone id; empty clears")
	if err := fs.Parse(rest); err != nil {
		return 1
	}
	if id == "" {
		return cliFail("feature-catalog update requires <id>")
	}
	set := setFlags(fs)
	patch := map[string]any{}
	if set["title"] {
		patch["title"] = *title
	}
	if set["description"] {
		patch["description"] = *description
	}
	if set["target-milestone"] {
		patch["targetMilestoneId"] = *target
	}
	if len(patch) == 0 {
		return cliFail("no updatable fields provided")
	}
	c, _, code := cliClient(*project)
	if code >= 0 {
		return code
	}
	return emitRaw(c.UpdateFeatureNode(id, patch))
}

func cliFeatureCatalogMove(args []string) int {
	id, rest := splitLeadingID(args)
	fs := flag.NewFlagSet("feature-catalog move", flag.ContinueOnError)
	project := fs.String("project", "", "project id|name|path")
	parent := fs.String("parent", "", "new parent module id; empty moves to root")
	position := fs.Int("position", 0, "zero-based sibling position")
	if err := fs.Parse(rest); err != nil {
		return 1
	}
	if id == "" {
		return cliFail("feature-catalog move requires <id>")
	}
	set := setFlags(fs)
	patch := map[string]any{}
	if set["parent"] {
		patch["parentId"] = *parent
	}
	if set["position"] {
		patch["position"] = *position
	}
	if len(patch) == 0 {
		return cliFail("move requires --parent and/or --position")
	}
	c, _, code := cliClient(*project)
	if code >= 0 {
		return code
	}
	return emitRaw(c.UpdateFeatureNode(id, patch))
}

func cliFeatureCatalogLink(args []string, unlink bool) int {
	featureID, rest := splitLeadingID(args)
	name := "feature-catalog link"
	if unlink {
		name = "feature-catalog unlink"
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	project := fs.String("project", "", "project id|name|path")
	item := fs.String("item", "", "project item id")
	relation := fs.String("relation", "", "source|delivery")
	if err := fs.Parse(rest); err != nil {
		return 1
	}
	if featureID == "" || strings.TrimSpace(*item) == "" {
		return cliFail("%s requires <feature-id> and --item", name)
	}
	if *relation != "source" && *relation != "delivery" {
		return cliFail("--relation must be source or delivery")
	}
	c, _, code := cliClient(*project)
	if code >= 0 {
		return code
	}
	if unlink {
		return emitRaw(c.UnlinkFeatureItem(featureID, *item, *relation))
	}
	return emitRaw(c.LinkFeatureItem(featureID, *item, *relation))
}

func cliFeatureCatalogBatch(args []string) int {
	fs := flag.NewFlagSet("feature-catalog batch", flag.ContinueOnError)
	project := fs.String("project", "", "project id|name|path")
	payload := fs.String("json", "", "JSON operations array")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	var operations json.RawMessage
	if strings.TrimSpace(*payload) == "" || json.Unmarshal([]byte(*payload), &operations) != nil {
		return cliFail("--json must be a valid JSON operations array")
	}
	var decoded []json.RawMessage
	if err := json.Unmarshal(operations, &decoded); err != nil || len(decoded) == 0 {
		return cliFail("--json must be a non-empty JSON operations array")
	}
	c, _, code := cliClient(*project)
	if code >= 0 {
		return code
	}
	return emitRaw(c.BatchFeatureCatalog(operations))
}

func cliFeatureCatalogGantt(args []string) int {
	fs := flag.NewFlagSet("feature-catalog gantt", flag.ContinueOnError)
	project := fs.String("project", "", "project id|name|path")
	_ = fs.Bool("json", false, "raw daemon JSON")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	c, _, code := cliClient(*project)
	if code >= 0 {
		return code
	}
	return emitRaw(c.FeatureCatalogGantt())
}

func cliFeatureCatalogExport(args []string) int {
	fs := flag.NewFlagSet("feature-catalog export", flag.ContinueOnError)
	project := fs.String("project", "", "project id|name|path")
	format := fs.String("format", "markdown", "export format: markdown or json")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *format != "markdown" && *format != "md" && *format != "json" {
		return cliFail("--format must be markdown or json")
	}
	c, _, code := cliClient(*project)
	if code >= 0 {
		return code
	}
	status, body, err := c.FeatureCatalogExport(*format)
	if err != nil {
		return cliFail("export error: %v", err)
	}
	if status != 200 {
		return cliFail("export failed: %d %s", status, string(body))
	}
	fmt.Print(string(body))
	return 0
}
