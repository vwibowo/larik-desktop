package integration

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestStarterSkillFrontmatter(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "examples", "larik-custom", "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(string(data), "---\n", 3)
	if len(parts) != 3 || parts[0] != "" {
		t.Fatal("starter skill needs YAML frontmatter")
	}
	fields := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(parts[1]), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			t.Fatalf("invalid frontmatter line %q", line)
		}
		fields[key] = strings.TrimSpace(value)
	}
	if !regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`).MatchString(fields["name"]) || len(fields["name"]) > 64 {
		t.Fatalf("invalid skill name %q", fields["name"])
	}
	if fields["description"] == "" || len(fields["description"]) > 1024 {
		t.Fatal("missing or oversized skill description")
	}
	if !strings.Contains(parts[2], "control list --json") {
		t.Fatal("starter skill has no control workflow")
	}
}
