package main

import (
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"go/build"
	"net/url"
)

func addImportsFlags(fs *flag.FlagSet) {
	// insecure is declared in fetch.go
	fs.BoolVar(&insecure, "precaire", false, "allow the use of insecure protocols")
}

var cmdImports = &Command{
	Name:      "imports",
	UsageLine: "imports [-precaire]",
	Short:     "read source imports and vendor all upstream dependencies",
	Long: `imports recursively reads imports from .go files and vendors upstream imports.

This command will rebuild the manifest file and remove everything in ./vendor/ if such a directory/files exist.
imports differs from fetch in that it can be used when trying to vendor all dependencies in an existing project and
it also works with imports which do not have manifest files. In such a case, the dependencies are recursively fetched
and added as a direct dependency to your project's manifest.

Flags:
	-precaire
		allow the use of insecure protocols.

`,
	Run: func(args []string) error {
		workingDir, err := os.Getwd()
		if err != nil {
			return err
		}

		return imports(workingDir)
	},
	AddFlags: addImportsFlags,
}

func imports(path string) error {
	// we use a map here to prevent adding duplicates
	imports := make(map[string]bool)

	// Recursively gather imports
	if err := importWorker(path, imports); err != nil {
		return err
	}

	var filteredImports []string

	for path, _ := range imports {
		if packageIsRemoteDependency(path) {
			filteredImports = append(filteredImports, path)
		}
	}

	fmt.Printf("%#v", filteredImports)

	return nil
}

// Import worker is the recursive call which does most of the work
// for gathering imports for a package
func importWorker(path string, imports map[string]bool) error {
	walkFunc := func(path string, f os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("Could not walk in dir %s: %s", path, err)
		}

		if !strings.HasSuffix(f.Name(), ".go") {
			return nil
		}

		fileImports, err := sourceFileImports(path)

		if err != nil {
			return err
		}

		for _, importedPackage := range fileImports {
			// we don't yet check if a package is remote dependency to avoid unnecessary
			// work for duplicates
			imports[importedPackage] = true
		}

		return nil
	}

	if err := filepath.Walk(path, walkFunc); err != nil {
		return err
	}

	return nil
}

// Parses the AST of Go source, gathers imports, and returns them
// source: https://golang.org/pkg/go/parser/#example_ParseFile
func sourceFileImports(path string) ([]string, error) {
	var imports []string

	fset := token.NewFileSet() // positions are relative to fset

	// parse the given file but stop after the imports
	f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}

	// Print the imports from the file's AST.
	for _, s := range f.Imports {
		imports = append(imports, strings.Trim(s.Path.Value, "\""))
	}

	return imports, nil
}

// Filters imports to only be remote dependencies
func packageIsRemoteDependency(name string) bool {
	fmt.Println(name)

	if build.IsLocalImport(name) {
		return false
	}

	// man, is this hacky. we'll say that this is temp until I decide to figure out "go get".
	// If we try using gbvendor's "DeduceRemoteRepo" method, we might be prompted
	// to enter github/bitbucket/etc. credentials. We don't actually want to probe anything, we just want to
	// see what might be a url
	url, err := url.Parse("http://" + name)
	if err != nil {
		return false
	}

	// check for the existence of a dot (TLD). told you this was hacky
	return strings.Contains(url.Host, ".")
}
