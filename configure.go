//go:build ignore

// Command configure turns this template into a package of your own.
//
// Run it once, from the root of a fresh clone:
//
//	go run ./configure.go
//
// It asks five questions, shows what it is going to replace, rewrites every
// file, renames the files and directories whose names carried a template value,
// formats the Go it touched, and deletes itself. There is nothing to undo
// afterwards and nothing left over to delete by hand.
//
// The build tag keeps it out of the package: this file is package main and the
// rest of the repository is not, and naming a file on the command line is what
// makes `go run` read it anyway. So `go build ./...`, `go vet ./...` and
// `go test ./...` all pass before it has ever run.
//
// It is safe to run twice, because it refuses to. Once the placeholders are
// gone there is nothing left to replace, and a second pass over a configured
// repository would rewrite whatever now happens to match. It says so and
// changes nothing.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// The values this template ships with. They are real, so the repository
// compiles and its tests pass before anything is configured, and they are what
// gets replaced in the Go sources and in go.mod -- neither of which can hold a
// placeholder that is not valid Go.
const (
	templateModulePath = "github.com/arandu-io/package-skeleton"
	templateSlug       = "skeleton"
	templateType       = "Skeleton"
)

// The placeholders, as they are spelled in prose: the README, the licence and
// the other documents, where a value that is not yet a value costs nothing.
const (
	markerModulePath     = ":module_path"
	markerModuleSlug     = ":module_slug"
	markerPackageName    = ":package_name"
	markerAuthorName     = ":author_name"
	markerAuthorUsername = ":author_username"
)

// The tokens that delimit a section belonging to the template rather than to
// the package. Whole lines from the first to the second are removed, so the
// same pair works in Markdown, in YAML and in Go.
const (
	sectionStart = "configure:template-start"
	sectionEnd   = "configure:template-end"
)

// selfName is this file, which is excluded from every rewrite and removed at
// the end.
const selfName = "configure.go"

// What each answer is allowed to be. A value that is refused here is a value
// that would produce a repository that does not compile, and finding that out
// now costs one prompt.
var (
	modulePathPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._~-]*(\.[a-zA-Z0-9._~-]+)+(/[a-zA-Z0-9._~-]+)+$`)
	// The slug is the package clause and part of every action name, so it is
	// held to what a Go identifier and a lowercase package name can both be.
	slugPattern     = regexp.MustCompile(`^[a-z][a-z0-9]*$`)
	usernamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`)
)

// answers is what the five questions produce, plus the one value derived from
// them.
type answers struct {
	modulePath     string
	moduleSlug     string
	packageName    string
	authorName     string
	authorUsername string
	// exportedType is the slug with its first letter capitalised. It is derived
	// rather than asked for, because a name that could disagree with the slug is
	// a name that eventually does.
	exportedType string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "configure:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		nonInteractive = flag.Bool("non-interactive", false, "take every answer from the flags below and ask nothing")
		yes            = flag.Bool("yes", false, "do not ask for confirmation before rewriting")
		modulePath     = flag.String("module-path", "", "the Go module path, which is what goes in go.mod")
		moduleSlug     = flag.String("module-slug", "", "the package name and the prefix of every action, lowercase")
		packageName    = flag.String("package-name", "", "the display name, for the README")
		authorName     = flag.String("author-name", "", "the author's name")
		authorUsername = flag.String("author-username", "", "the author's account name")
	)
	flag.Parse()

	root, err := repositoryRoot()
	if err != nil {
		return err
	}

	files, err := readTextFiles(root)
	if err != nil {
		return err
	}
	if !carriesPlaceholders(files) {
		return errors.New("this repository carries no placeholders, so it has already been configured. " +
			"Nothing was changed: a second pass would rewrite whatever now happens to match. " +
			"Delete " + selfName + ", or start again from a fresh clone of the template")
	}

	given := answers{
		modulePath:     *modulePath,
		moduleSlug:     *moduleSlug,
		packageName:    *packageName,
		authorName:     *authorName,
		authorUsername: *authorUsername,
	}

	var chosen answers
	if *nonInteractive {
		chosen, err = fromFlags(given)
	} else {
		chosen, err = ask(given)
	}
	if err != nil {
		return err
	}

	pairs := replacements(chosen)
	report(chosen, pairs)

	if !*nonInteractive && !*yes {
		ok, err := confirm()
		if err != nil {
			return err
		}
		if !ok {
			fmt.Println("nothing was changed")
			return nil
		}
	}

	// Everything is computed before anything is written, and the Go is formatted
	// in memory. A file that would not parse stops the run with the tree
	// untouched, rather than half of it rewritten.
	rewritten, err := rewriteAll(files, pairs)
	if err != nil {
		return err
	}
	for _, file := range rewritten {
		if file.content == file.original {
			continue
		}
		if err := os.WriteFile(file.path, []byte(file.content), file.mode); err != nil {
			return fmt.Errorf("writing %s: %w", file.path, err)
		}
	}

	// Names last, because everything above addressed files by the path they
	// still had.
	if err := renamePaths(root, pairs); err != nil {
		return err
	}

	if err := os.Remove(filepath.Join(root, selfName)); err != nil {
		return fmt.Errorf("removing %s: %w", selfName, err)
	}

	fmt.Println()
	fmt.Println("done. " + selfName + " has removed itself.")
	fmt.Println("Next: go build ./... && go test ./...")
	return nil
}

// repositoryRoot walks up from the working directory to the nearest directory
// holding both go.mod and this file.
//
// Both, and not go.mod alone: a clone sitting inside another Go module would
// otherwise be configured by walking up into a repository nobody meant to
// touch.
func repositoryRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		_, modErr := os.Stat(filepath.Join(dir, "go.mod"))
		_, selfErr := os.Stat(filepath.Join(dir, selfName))
		if modErr == nil && selfErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no go.mod with a " + selfName + " beside it was found here or above. Run this from the root of a clone of the template")
		}
		dir = parent
	}
}

// file is one text file, as read.
type file struct {
	path     string
	rel      string
	mode     fs.FileMode
	original string
	content  string
}

// readTextFiles reads every text file in the tree, and skips what a rewrite has
// no business touching.
//
// go.sum is skipped by name rather than by content: it is text, it is
// machine-written, and a hash that happened to contain a word being replaced
// would be silently corrupted. `go mod tidy` rewrites it anyway once the module
// path changes.
func readTextFiles(root string) ([]*file, error) {
	var out []*file

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if entry.IsDir() {
			if name == ".git" || name == "node_modules" {
				return fs.SkipDir
			}
			return nil
		}
		if name == selfName || name == "go.sum" {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		// Four megabytes is far past anything in a package repository, and
		// reading the whole tree into memory is what lets the run be decided
		// before a byte is written.
		if info.Size() > 4<<20 {
			return nil
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// A NUL byte is the one reliable sign of something that is not text.
		// An image or a font rewritten as if it were would be destroyed.
		if strings.IndexByte(string(raw), 0) >= 0 {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out = append(out, &file{
			path: path, rel: filepath.ToSlash(rel), mode: info.Mode().Perm(),
			original: string(raw), content: string(raw),
		})
		return nil
	})
	return out, err
}

// carriesPlaceholders reports whether anything is left to replace.
//
// It looks at the files rather than at this one, which still holds every
// placeholder as a string constant. Restoring this file into a configured
// repository and running it again therefore finds nothing, which is the answer.
func carriesPlaceholders(files []*file) bool {
	tokens := []string{
		templateModulePath, templateSlug, templateType,
		markerModulePath, markerModuleSlug, markerPackageName,
		markerAuthorName, markerAuthorUsername,
	}
	for _, f := range files {
		for _, token := range tokens {
			if strings.Contains(f.original, token) {
				return true
			}
		}
	}
	return false
}

// fromFlags validates the answers a pipeline passed in.
func fromFlags(given answers) (answers, error) {
	for _, required := range []struct {
		flag  string
		value string
	}{
		{"module-path", given.modulePath},
		{"module-slug", given.moduleSlug},
		{"package-name", given.packageName},
		{"author-name", given.authorName},
		{"author-username", given.authorUsername},
	} {
		if strings.TrimSpace(required.value) == "" {
			return answers{}, fmt.Errorf("-non-interactive needs -%s", required.flag)
		}
	}

	out := answers{
		modulePath:     strings.TrimSpace(given.modulePath),
		moduleSlug:     strings.TrimSpace(given.moduleSlug),
		packageName:    strings.TrimSpace(given.packageName),
		authorName:     strings.TrimSpace(given.authorName),
		authorUsername: strings.TrimSpace(given.authorUsername),
	}
	if err := validate(out); err != nil {
		return answers{}, err
	}
	out.exportedType = exported(out.moduleSlug)
	return out, nil
}

// ask puts the five questions, one at a time, with a default where one can be
// derived from an answer already given.
func ask(given answers) (answers, error) {
	reader := bufio.NewReader(os.Stdin)
	out := answers{}
	var err error

	fmt.Println("This turns the template into a package of your own. Five questions.")
	fmt.Println()

	if out.authorUsername, err = prompt(reader, "author username", given.authorUsername, ""); err != nil {
		return answers{}, err
	}
	if out.authorName, err = prompt(reader, "author name", given.authorName, ""); err != nil {
		return answers{}, err
	}
	if out.modulePath, err = prompt(reader, "module path", given.modulePath,
		"github.com/"+out.authorUsername+"/arandu-thing ..."); err != nil {
		return answers{}, err
	}
	if out.moduleSlug, err = prompt(reader, "module slug", given.moduleSlug, slugFrom(out.modulePath)); err != nil {
		return answers{}, err
	}
	if out.packageName, err = prompt(reader, "package name", given.packageName, exported(out.moduleSlug)); err != nil {
		return answers{}, err
	}

	if err := validate(out); err != nil {
		return answers{}, err
	}
	out.exportedType = exported(out.moduleSlug)
	return out, nil
}

// prompt reads one answer, offering a default when there is a usable one.
//
// A suggestion that would not pass validation is offered as a hint rather than
// as a default: pressing return has to produce something that works.
func prompt(reader *bufio.Reader, question, given, suggestion string) (string, error) {
	if strings.TrimSpace(given) != "" {
		return strings.TrimSpace(given), nil
	}

	usable := suggestion != "" && !strings.Contains(suggestion, "...")
	label := question
	if usable {
		label += " [" + suggestion + "]"
	} else if suggestion != "" {
		label += " (for example " + suggestion + ")"
	}

	for {
		fmt.Print(label + ": ")
		line, err := reader.ReadString('\n')
		answer := strings.TrimSpace(line)
		if answer == "" && usable {
			return suggestion, nil
		}
		if answer != "" {
			return answer, nil
		}
		if err != nil {
			return "", fmt.Errorf("reading the answer to %q: %w", question, err)
		}
	}
}

// validate refuses an answer that would produce a repository that does not
// compile, or one whose package clause is not a package clause.
func validate(a answers) error {
	if !modulePathPattern.MatchString(a.modulePath) {
		return fmt.Errorf("the module path %q is not one: it looks like github.com/you/arandu-thing", a.modulePath)
	}
	if !slugPattern.MatchString(a.moduleSlug) {
		return fmt.Errorf("the module slug %q cannot be a package name: lowercase letters and digits, starting with a letter", a.moduleSlug)
	}
	if !usernamePattern.MatchString(a.authorUsername) {
		return fmt.Errorf("the author username %q is not one: letters, digits and -", a.authorUsername)
	}
	if strings.TrimSpace(a.authorName) == "" {
		return errors.New("the author name is required")
	}
	if strings.TrimSpace(a.packageName) == "" {
		return errors.New("the package name is required")
	}
	return nil
}

// slugFrom derives a slug from a module path: the last element, with a
// conventional prefix removed and everything a package name cannot hold
// dropped.
func slugFrom(modulePath string) string {
	last := modulePath
	if i := strings.LastIndex(modulePath, "/"); i >= 0 {
		last = modulePath[i+1:]
	}
	last = strings.ToLower(last)
	for _, prefix := range []string{"arandu-", "go-"} {
		last = strings.TrimPrefix(last, prefix)
	}
	var b strings.Builder
	for _, r := range last {
		letter := r >= 'a' && r <= 'z'
		// A digit is kept only once a letter has been written, because a package
		// name cannot start with one.
		digit := r >= '0' && r <= '9' && b.Len() > 0
		if letter || digit {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// exported is the slug as an exported Go identifier.
func exported(slug string) string {
	if slug == "" {
		return ""
	}
	return strings.ToUpper(slug[:1]) + slug[1:]
}

// pair is one replacement.
type pair struct{ old, new string }

// replacements is every substitution, longest first.
//
// The order matters and the length is what settles it. The template module path
// ends in the template slug, so replacing the slug first would leave a module
// path nothing matches any more. Sorting by length puts the whole path ahead of
// the word inside it, and rewrite never looks at what it has already written.
func replacements(a answers) []pair {
	out := []pair{
		{templateModulePath, a.modulePath},
		{markerModulePath, a.modulePath},
		{markerModuleSlug, a.moduleSlug},
		{markerPackageName, a.packageName},
		{markerAuthorName, a.authorName},
		{markerAuthorUsername, a.authorUsername},
		{templateType, a.exportedType},
		{templateSlug, a.moduleSlug},
	}
	sort.SliceStable(out, func(i, j int) bool { return len(out[i].old) > len(out[j].old) })
	return out
}

// rewrite is one left-to-right pass, taking the first match at each position.
//
// A pass is what keeps the result independent of what the replacements produce:
// a sequence of whole-file substitutions would let the second one rewrite what
// the first one just wrote, which is how a module path ends up half translated.
func rewrite(text string, pairs []pair) string {
	var out strings.Builder
	out.Grow(len(text))
	for i := 0; i < len(text); {
		matched := false
		for _, p := range pairs {
			if strings.HasPrefix(text[i:], p.old) {
				out.WriteString(p.new)
				i += len(p.old)
				matched = true
				break
			}
		}
		if !matched {
			out.WriteByte(text[i])
			i++
		}
	}
	return out.String()
}

// stripTemplateSections removes the parts that belong to the template rather
// than to the package: the instructions for running this command, and the CI
// job that exercises it.
//
// Whole lines, from the opening token to the closing one, so the same pair of
// markers works in Markdown, in YAML and in Go. An opening token with no
// closing one is a mistake in the template and is reported rather than guessed
// at: dropping the rest of the file would be the alternative.
func stripTemplateSections(rel, text string) (string, error) {
	if !strings.Contains(text, sectionStart) {
		return text, nil
	}

	var out []string
	inside := false
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.Contains(line, sectionStart):
			if inside {
				return "", fmt.Errorf("%s: a %s opens inside another one", rel, sectionStart)
			}
			inside = true
		case strings.Contains(line, sectionEnd):
			if !inside {
				return "", fmt.Errorf("%s: a %s closes a section that never opened", rel, sectionEnd)
			}
			inside = false
		case !inside:
			out = append(out, line)
		}
	}
	if inside {
		return "", fmt.Errorf("%s: a %s is never closed", rel, sectionStart)
	}

	// A removed section leaves the blank line that separated it from what came
	// next, next to the one that separated what came before -- so runs of blank
	// lines are collapsed to one. Removing all of them instead would reflow
	// files nobody edited.
	text = strings.Join(out, "\n")
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}
	return text, nil
}

// rewriteAll computes the new content of every file, and formats the Go.
//
// Nothing is written here. The formatter is the same one gofmt is, so a
// substitution that produced Go which does not parse fails the whole run with
// the tree untouched -- rather than leaving a repository that has to be thrown
// away.
func rewriteAll(files []*file, pairs []pair) ([]*file, error) {
	for _, f := range files {
		text, err := stripTemplateSections(f.rel, f.original)
		if err != nil {
			return nil, err
		}
		text = rewrite(text, pairs)

		if strings.HasSuffix(f.rel, ".go") && !strings.HasSuffix(f.rel, ".kyse.go") {
			formatted, err := format.Source([]byte(text))
			if err != nil {
				return nil, fmt.Errorf("%s does not parse after the substitution, so nothing was written: %w", f.rel, err)
			}
			text = string(formatted)
		}
		f.content = text
	}
	return files, nil
}

// renamePaths applies the same substitutions to the names of files and
// directories.
//
// Contents alone is not enough, because one name in this tree is read as data.
// A skill under .agents/skills/ declares its own directory name in its
// frontmatter, and a tool that reads the two and finds them different skips the
// skill -- so a package configured with the contents rewritten and the directory
// left alone would ship a skill nothing loads, and nothing in the build would
// say so.
//
// Deepest first, so renaming a directory never invalidates a path that is still
// waiting to be renamed underneath it.
func renamePaths(root string, pairs []pair) error {
	var targets []string

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if entry.IsDir() && (name == ".git" || name == "node_modules") {
			return fs.SkipDir
		}
		if path != root && rewrite(name, pairs) != name {
			targets = append(targets, path)
		}
		return nil
	})
	if err != nil {
		return err
	}

	sort.SliceStable(targets, func(i, j int) bool {
		return strings.Count(targets[i], string(filepath.Separator)) >
			strings.Count(targets[j], string(filepath.Separator))
	})

	for _, path := range targets {
		dir, name := filepath.Split(path)
		renamed := filepath.Join(dir, rewrite(name, pairs))
		// A name already taken is reported rather than overwritten. The rename
		// stops here, and what has already been renamed stays renamed: the
		// alternative is a silent replacement of a file somebody wrote.
		if _, err := os.Lstat(renamed); err == nil {
			return fmt.Errorf("renaming %s to %s: something is already there, and the rename stopped at that point", path, renamed)
		}
		if err := os.Rename(path, renamed); err != nil {
			return fmt.Errorf("renaming %s: %w", path, err)
		}
	}
	return nil
}

// report prints what is about to happen, before it happens.
func report(a answers, pairs []pair) {
	fmt.Println()
	fmt.Println("about to replace, everywhere in this repository -- in the contents of every")
	fmt.Println("file, and in the names of the files and directories that carry one:")
	for _, p := range pairs {
		fmt.Printf("  %-38s -> %s\n", p.old, p.new)
	}
	fmt.Println()
	fmt.Println("and then remove " + selfName + " and the sections marked as belonging to the template.")
}

// confirm asks once, and takes anything other than yes for no.
func confirm() (bool, error) {
	fmt.Print("go ahead? [y/N]: ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		return false, nil
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}
