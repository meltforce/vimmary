package vimmary

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// ciJob matches a job key in a workflow file: a name indented by exactly two
// spaces. Only the part after the top-level "jobs:" line is searched.
var ciJob = regexp.MustCompile(`(?m)^  ([A-Za-z0-9_-]+):\s*$`)

// TestReadmeNamesCIJobs keeps the README's list of CI checks in step with
// the jobs .github/workflows/ci.yml defines.
func TestReadmeNamesCIJobs(t *testing.T) {
	workflow, err := os.ReadFile(".github/workflows/ci.yml")
	if err != nil {
		t.Fatalf("read workflow: %v", err)
	}
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("read README: %v", err)
	}

	_, jobs, found := strings.Cut(string(workflow), "\njobs:\n")
	if !found {
		t.Fatal("ci.yml has no top-level jobs section")
	}
	matches := ciJob.FindAllStringSubmatch(jobs, -1)
	if len(matches) == 0 {
		t.Fatal("found no jobs in ci.yml")
	}
	for _, m := range matches {
		if name := m[1]; !strings.Contains(string(readme), "`"+name+"`") {
			t.Errorf("README.md does not name the CI job `%s`", name)
		}
	}
}
