package unit_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestTheManifestFrameworkFloorMatchesGoMod keeps the two places that state a
// Framework version from disagreeing. go.mod is what a consumer compiles
// against; arandu.mod.toml is what `aru doctor` reads in the application that
// installs this package. A floor lower than the compiled version is a promise
// the code cannot keep, and nothing else compares them.
func TestTheManifestFrameworkFloorMatchesGoMod(t *testing.T) {
	t.Parallel()

	root := packageRoot(t)
	required := captureReleaseValue(t, readReleaseFile(t, root, "go.mod"),
		`(?m)^\s*github\.com/arandu-io/framework v([0-9]+\.[0-9]+)\.[0-9]+\s*$`,
		"Framework version in go.mod")
	declared := captureReleaseValue(t, readReleaseFile(t, root, "arandu.mod.toml"),
		`(?m)^framework = ">= ([0-9]+\.[0-9]+)"$`,
		"Framework floor in arandu.mod.toml")

	if declared != required {
		t.Fatalf("manifest Framework floor = %s, want %s from go.mod", declared, required)
	}
}

// TestTheGuidesUseTheManifestFrameworkFloor holds the prose to the manifest.
// The README states the requirement a reader installs against and the release
// skill reproduces the manifest verbatim, so a bump that stops at go.mod leaves
// both documents naming a version this package no longer supports.
func TestTheGuidesUseTheManifestFrameworkFloor(t *testing.T) {
	t.Parallel()

	root := packageRoot(t)
	declared := captureReleaseValue(t, readReleaseFile(t, root, "arandu.mod.toml"),
		`(?m)^framework = ">= ([0-9]+\.[0-9]+)"$`,
		"Framework floor in arandu.mod.toml")

	guides := map[string]string{
		"README.md": "Arandu Framework " + declared + " or newer.",
		".agents/skills/whatsapp-release/SKILL.md": `framework = ">= ` + declared + `"`,
	}
	for guide, want := range guides {
		if !strings.Contains(readReleaseFile(t, root, guide), want) {
			t.Errorf("%s does not state the manifest floor %q", guide, want)
		}
	}
}

func readReleaseFile(t *testing.T, root, name string) string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return string(raw)
}

func captureReleaseValue(t *testing.T, body, pattern, label string) string {
	t.Helper()

	match := regexp.MustCompile(pattern).FindStringSubmatch(body)
	if len(match) != 2 {
		t.Fatalf("%s is missing", label)
	}
	return match[1]
}
