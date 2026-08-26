package unit_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImplementationUsesNativeAranduResponsibilityTree(t *testing.T) {
	t.Parallel()
	root := packageRoot(t)

	for _, path := range []string{
		"app/Enums",
		"app/Http/Controllers",
		"app/Http/Documentation",
		"app/Http/Requests",
		"app/Http/Resources",
		"app/Jobs",
		"app/Models",
		"app/Policies",
		"app/Repositories",
		"app/Services",
		"config",
		"database/migrations",
		"routes",
	} {
		entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Errorf("native directory %s is unavailable: %v", path, err)
			continue
		}
		hasGo := false
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
				hasGo = true
				break
			}
		}
		if !hasGo {
			t.Errorf("native directory %s contains no Go implementation", path)
		}
	}

	for _, legacy := range []string{
		"config.go", "model.go", "policy.go", "repository.go", "service.go",
		"routes.go", "handler_support.go", "handlers_chat_group.go",
		"handlers_instances.go", "handlers_messages.go", "migrations.go",
	} {
		_, err := os.Stat(filepath.Join(root, legacy))
		if err == nil {
			t.Errorf("legacy root implementation %s still exists", legacy)
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("inspect legacy root implementation %s: %v", legacy, err)
		}
	}
}

func TestControllersCannotReachRepositories(t *testing.T) {
	t.Parallel()
	root := packageRoot(t)
	controllers := filepath.Join(root, "app", "Http", "Controllers")
	entries, err := os.ReadDir(controllers)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(controllers, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"github.com/hyz-is/arandu-whatsapp/app/Repositories",
			"github.com/hyz-is/arandu-whatsapp/internal/database/repository",
		} {
			if strings.Contains(string(body), forbidden) {
				t.Errorf("%s reaches a repository through %s", entry.Name(), forbidden)
			}
		}
	}
}

func TestApplicationLayersDoNotReachInternalPersistence(t *testing.T) {
	t.Parallel()
	root := packageRoot(t)
	for _, directory := range []string{
		"app/Http/Controllers",
		"app/Models",
		"app/Policies",
		"app/Services",
	} {
		entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(directory)))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
				continue
			}
			body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(directory), entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{
				"github.com/hyz-is/arandu-whatsapp/internal/database/repository",
				"github.com/hyz-is/arandu-whatsapp/internal/database/types",
			} {
				if strings.Contains(string(body), forbidden) {
					t.Errorf("%s/%s reaches persistence through %s", directory, entry.Name(), forbidden)
				}
			}
		}
	}
}

func TestImplementationPackagesDoNotImportTheRootFacade(t *testing.T) {
	t.Parallel()
	root := packageRoot(t)
	for _, directory := range []string{"app", "config", "database", "routes"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(body), `"github.com/hyz-is/arandu-whatsapp"`) {
				relative, _ := filepath.Rel(root, path)
				t.Errorf("implementation package %s imports the root compatibility facade", filepath.ToSlash(relative))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func packageRoot(t *testing.T) string {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(workingDirectory, "..", ".."))
}
