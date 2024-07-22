package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
)

var (
	customTestingName = "customFuzzTestingPkg"

	buildFlags2 = []string{
		"-buildmode", "c-archive",
		"-trimpath",
		"-gcflags", "all=-d=libfuzzer",
	}

	stdLibPkgs = []string{"testing", "os", "reflect", "math/rand", "testing/internal/testdeps"}
)

type FileWalker struct {
	renamedFiles map[string]string
}

func NewFileWalker() *FileWalker {
	return &FileWalker {
		renamedFiles: make(map[string]string),
	}
}



// rewriteTestingImports rewrites imports for:
// - all package files
// - the fuzzer
// - dependencies
//
// it rewrites "testing" => "github.com/AdamKorcz/go-118-fuzz-build/testing"
func rewriteTestingImports(pkgs []*packages.Package, fuzzName string) (string, []byte, error) {
	var fuzzFilepath string
	var originalFuzzContents []byte
	originalFuzzContents = []byte("NONE")

	// First find file with fuzz harness
	for _, pkg := range pkgs {
		for _, file := range pkg.GoFiles {
			//fmt.Println(file)
			err := rewriteTestingImport(file)
			if err != nil {
				panic(err)
			}
		}
	}

	// rewrite testing in imported packages
	packages.Visit(pkgs, rewriteImportTesting, nil)

	for _, pkg := range pkgs {
		for _, file := range pkg.GoFiles {
			fuzzFile, b, err := rewriteFuzzer(file, fuzzName)
			if err != nil {
				panic(err)
			}
			if fuzzFile != "" {
				fuzzFilepath = fuzzFile
				originalFuzzContents = b
			}
		}
	}
	return fuzzFilepath, originalFuzzContents, nil
}

func rewriteFuzzer(path, fuzzerName string) (originalPath string, originalFile []byte, err error) {
	var fileHasOurHarness bool // to determine whether we should rewrite filename
	fileHasOurHarness = false

	var originalFuzzContents []byte
	originalFuzzContents = []byte("NONE")

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return "", originalFuzzContents, err
	}
	for _, decl := range f.Decls {
		if _, ok := decl.(*ast.FuncDecl); ok {
			if decl.(*ast.FuncDecl).Name.Name == fuzzerName {
				fileHasOurHarness = true

			}
		}
	}

	if fileHasOurHarness {
		originalFuzzContents, err = os.ReadFile(path)
		if err != nil {
			panic(err)
		}

		// Replace import path
		astutil.DeleteImport(fset, f, "testing")
		astutil.AddImport(fset, f, "github.com/AdamKorcz/go-118-fuzz-build/testing")
	}

	// Rewrite filename
	if fileHasOurHarness {
		var buf bytes.Buffer
		printer.Fprint(&buf, fset, f)

		newFile, err := os.Create(path + "_fuzz.go")
		if err != nil {
			panic(err)
		}
		defer newFile.Close()
		newFile.Write(buf.Bytes())
		return path, originalFuzzContents, nil
	}
	return "", originalFuzzContents, nil
}

// Rewrites testing import of a single path
func rewriteTestingImport(path string) error {
	//fmt.Println("Rewriting ", path)
	fsetCheck := token.NewFileSet()
	fCheck, err := parser.ParseFile(fsetCheck, path, nil, parser.ImportsOnly)
	if err != nil {
		return err
	}

	// First check if the import already exists
	// Return if it does.
	for _, imp := range fCheck.Imports {
		if imp.Path.Value == "github.com/AdamKorcz/go-118-fuzz-build/testing" {
			return nil
		}
	}

	// Replace import path
	for _, imp := range fCheck.Imports {
		if imp.Path.Value == "testing" {
			imp.Path.Value = "github.com/AdamKorcz/go-118-fuzz-build/testing"
		}
	}
	return nil
}

// Rewrites testing import of a package
func rewriteImportTesting(pkg *packages.Package) bool {
	for _, file := range pkg.GoFiles {
		err := rewriteTestingImport(file)
		if err != nil {
			panic(err)
		}
	}
	return true
}

// Checks whether a fuzz test exists in a given file
func rewriteTestingFFunctionParams(path string) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return err
	}
	update := false
	for _, decl := range f.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok {
			for _, param := range funcDecl.Type.Params.List {
				fmt.Println(param.Names)
				if paramType, ok := param.Type.(*ast.StarExpr); ok {
					if p2, ok := paramType.X.(*ast.SelectorExpr); ok {
						if p3, ok := p2.X.(*ast.Ident); ok {
							if p3.Name == "testing" && p2.Sel.Name == "F" {
								p3.Name = customTestingName
								update = true
							}
						}
					}
				}
			}
		}
	}
	if update {
		var buf bytes.Buffer
		printer.Fprint(&buf, fset, f)

		newFile, err := os.Create(path)
		if err != nil {
			panic(err)
		}
		defer newFile.Close()
		newFile.Write(buf.Bytes())
	}
	return nil
}

func (walker *FileWalker) RewriteAllImportedTestFiles(files []string) error {
	for _, file := range files {
		if file[len(file)-8:] == "_test.go" {
			newName := strings.TrimSuffix(file, "_test.go")+"_libFuzzer.go"
			err := os.Rename(file, newName)
			if err != nil {
				return err
			}
			walker.addRenamedFile(file, newName)
		}
	}
	return nil
}

func (walker *FileWalker) addRenamedFile(oldPath, newPath string) {
	if _, ok := walker.renamedFiles[oldPath]; ok {
		panic("The file already exists which it shouldn't")
	}
	walker.renamedFiles[oldPath] = newPath
}

/* Gets a list of files that are imported by a file */
func GetAllSourceFilesOfFile(filePath string) ([]string, error) {
	files := make([]string, 0)
	pkgs, err := getAllPackagesOfFile(filePath)
	if err != nil {
		return files, err
	}
	for _, pkg := range pkgs {
		for _, file := range pkg.GoFiles {
			// There may be compiled files in the go cache. Ignore those
			if strings.Contains(file, "/.cache/") {
				continue
			}
			files = append(files, file)
		}
	}
	return files, nil
}

func getAllPackagesOfFile(filePath string) ([]*packages.Package, error) {
	pkgs, err := packages.Load(&packages.Config{
		Mode:       LoadMode,
		BuildFlags: buildFlags2,
		Tests:      true,
	}, "file="+filePath)
	if err != nil {
		return pkgs, err
	}
	err = os.Chdir(filepath.Dir(filePath))
	if err != nil {
		return pkgs, err
	}
	// There should only be one file
	if len(pkgs) != 1 {
		panic("there should only be one file here")
	}
	return appendPkgImports(pkgs[0], pkgs)
}

func appendPkgImports(pkg *packages.Package, pkgs []*packages.Package) ([]*packages.Package, error) {
	pkgsCopy := pkgs
	for _, imp := range pkg.Imports {
		if isStdLibPkg(imp.PkgPath) {
			continue
		}
		p, err := loadPkg(imp.PkgPath)
		if err != nil {
			return pkgsCopy, err
		}
		for _, pack := range p {
			if pkgInPkgs(pack.PkgPath, pkgsCopy) {
				continue
			}
			pkgsCopy = append(pkgsCopy, pack)
			pkgsCopy, err = appendPkgImports(pack, pkgsCopy)
			if err != nil {
				return pkgsCopy, err
			}
		}
	}
	return pkgsCopy, nil
}

func loadPkg(path string) ([]*packages.Package, error) {
	pkgs, err := packages.Load(&packages.Config{
		Mode:       LoadMode,
		BuildFlags: buildFlags2,
		Tests:      true,
	}, path)
	if err != nil {
		return pkgs, err
	}
	return pkgs, nil
}

func pkgInPkgs(importPath string, pkgs []*packages.Package) bool {
	for _, pkg := range pkgs {
		if strings.EqualFold(pkg.PkgPath, importPath) {
			return true
		}
	}
	return false
}

func isStdLibPkg(importName string) bool {
	for _, stdLibPkg := range stdLibPkgs {
		if strings.EqualFold(importName, stdLibPkg) {
			return true
		}
	}
	return false
}
