package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/docker/docker/daemon/graphdriver/copy"
)

func TestRewriteFuncTestingFParams(t *testing.T) {
	fileContents := `package main
import (
	"testing"
)
func ourTestHelper(f *testing.F) {
	_ = f
}`
	expectedFileContents := `package main

import (
	"testing"
)

func ourTestHelper(f *customFuzzTestingPkg.F) {
	_ = f
}
`
	file := filepath.Join(t.TempDir(), "file.go")
	err := os.WriteFile(file, []byte(fileContents), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	rewriteTestingFFunctionParams(file)
	gotFileContents, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotFileContents, []byte(expectedFileContents)) {
		t.Errorf("%s", cmp.Diff(gotFileContents, []byte(expectedFileContents)))
	}
}

func TestGetAllPackagesOfFile(t *testing.T) {
	pkgs, err := getAllPackagesOfFile(filepath.Join("testdata", "module1", "fuzz_test.go"))
	if err != nil {
		t.Fatalf("failed to load packages: %s", err)
	}
	if pkgs[0].Name != "module1" {
		t.Error("pkgs[0].Name should be 'module1'")
	}
	if pkgs[1].Name != "submodule1" {
		t.Error("pkgs[1].Name should be 'submodule1'")
	}
	if pkgs[2].Name != "submodule2" {
		t.Error("pkgs[2].Name should be 'submodule2'")
	}
	if pkgs[3].Name != "submodule1_test" {
		t.Error("pkgs[3].Name should be 'submodule1_test'")
	}
	if pkgs[4].Name != "main" {
		t.Error("pkgs[4].Name should be 'main'")
	}
}

func TestGetAllSourceFilesOfFile(t *testing.T) {
	files, err := GetAllSourceFilesOfFile(filepath.Join("testdata", "module1", "fuzz_test.go"))
	if err != nil {
		t.Fatalf("failed to load packages: %s", err)
	}
	if filepath.Base(files[0]) != "fuzz_test.go" {
		t.Error("files[0] should be 'fuzz_test.go'")
	}
	if filepath.Base(files[1]) != "one.go" {
		t.Error("files[1] should be 'one.go'")
	}
	if filepath.Base(files[2]) != "test_one.go" {
		t.Error("files[2] should be 'test_one.go'")
	}
	if filepath.Base(files[3]) != "one_test.go" {
		t.Error("files[3] should be 'one_test.go'")
	}
}

func TestRenameAllTestFiles(t *testing.T) {
	tempDir := t.TempDir()
	err := copy.DirCopy(filepath.Join("testdata", "module1"),
						filepath.Join(tempDir, "module1"),
						copy.Content,
						false)
	if err != nil {
		t.Fatal(err)
	}
	files, err := GetAllSourceFilesOfFile(filepath.Join(tempDir, "module1", "fuzz_test.go"))
	if err != nil {
		t.Fatalf("failed to load packages: %s", err)
	}
	_, err = os.Stat(filepath.Join(tempDir, "module1", "fuzz_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = os.Stat(filepath.Join(tempDir, "module1", "submodule1", "one.go"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = os.Stat(filepath.Join(tempDir, "module1", "submodule1", "one_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = os.Stat(filepath.Join(tempDir, "module1", "submodule2", "test_one.go"))
	if err != nil {
		t.Fatal(err)
	}
	walker := NewFileWalker()
	walker.RewriteAllImportedTestFiles(files)

	_, err = os.Stat(filepath.Join(tempDir, "module1", "fuzz_libFuzzer.go"))
	if err != nil {
		t.Error("Did not rewrite module1/fuzz_test.go")
	}
	_, err = os.Stat(filepath.Join(tempDir, "module1", "submodule1", "one.go"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = os.Stat(filepath.Join(tempDir, "module1", "submodule1", "one_libFuzzer.go"))
	if err != nil {
		t.Fatal("Did not rewrite module1/submodule1/one_test.go")
	}
	_, err = os.Stat(filepath.Join(tempDir, "module1", "submodule2", "test_one.go"))
	if err != nil {
		t.Fatal(err)
	}
	
	fmt.Println(walker.renamedFiles)
	if len(walker.renamedFiles) != 2 {
		t.Error("There should be two rewrites")
	}

	if fuzzTest, ok := walker.renamedFiles[filepath.Join(tempDir, "module1", "fuzz_test.go")]; ok {
		if fuzzTest != filepath.Join(tempDir, "module1", "fuzz_libFuzzer.go") {
			t.Errorf("Path is %s but should be %s", fuzzTest, filepath.Join(tempDir, "module1", "fuzz_libFuzzer.go"))
		}
	}

	if oneTest, ok := walker.renamedFiles[filepath.Join(tempDir, "module1", "submodule1", "one_test.go")]; ok {
		if oneTest != filepath.Join(tempDir, "module1", "submodule1", "one_libFuzzer.go") {
			t.Errorf("Path is %s but should be %s", oneTest, filepath.Join(tempDir, "module1", "submodule1", "one_libFuzzer.go"))
		}
	}
}
