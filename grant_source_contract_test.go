package whatsapp

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrantFirstSourceContract(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "testdata", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(source)
		for _, forbidden := range []string{"WithGrant(", "grantFromContext"} {
			if strings.Contains(text, forbidden) {
				t.Errorf("%s contains forbidden hidden-Grant pattern %q", path, forbidden)
			}
		}
		if strings.HasPrefix(filepath.ToSlash(path), "internal/database/repository/") && strings.Contains(text, "context.WithValue(") {
			t.Errorf("%s stores repository authority in a context", path)
		}
		checkSystemGrantMarkers(t, path, text)

		file, err := parser.ParseFile(fset, path, source, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			position := fset.Position(call.Pos())
			if selector.Sel.Name == "Check" && len(call.Args) == 1 && !isActionConstant(call.Args[0]) {
				t.Errorf("%s:%d passes a dynamic value to Grant.Check", path, position.Line)
			}
			if selector.Sel.Name == "HasPrefix" && expressionCallsAction(call) {
				t.Errorf("%s:%d authorizes by action prefix", path, position.Line)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func checkSystemGrantMarkers(t *testing.T, path string, source string) {
	t.Helper()
	clean := filepath.ToSlash(path)
	if strings.HasPrefix(clean, "cmd/") || strings.Contains(clean, "/jobs/") {
		return
	}

	scanner := bufio.NewScanner(strings.NewReader(source))
	previous := ""
	line := 0
	for scanner.Scan() {
		line++
		current := scanner.Text()
		if strings.Contains(current, "SystemGrant(") &&
			!validSystemGrantMarker(current) && !validSystemGrantMarker(previous) {
			t.Errorf("%s:%d creates a SystemGrant without an Aru boundary marker", path, line)
		}
		previous = current
	}
	if err := scanner.Err(); err != nil {
		t.Errorf("scan %s: %v", path, err)
	}
}

func validSystemGrantMarker(line string) bool {
	const marker = "//arandu:system-grant "
	index := strings.Index(line, marker)
	return index >= 0 && strings.TrimSpace(line[index+len(marker):]) != ""
}

func isActionConstant(expression ast.Expr) bool {
	switch value := expression.(type) {
	case *ast.Ident:
		return strings.HasPrefix(value.Name, "Action")
	case *ast.SelectorExpr:
		return strings.HasPrefix(value.Sel.Name, "Action")
	default:
		return false
	}
}

func expressionCallsAction(node ast.Node) bool {
	found := false
	ast.Inspect(node, func(candidate ast.Node) bool {
		call, ok := candidate.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if ok && selector.Sel.Name == "Action" {
			found = true
			return false
		}
		return true
	})
	return found
}
