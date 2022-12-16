# Go-118-fuzz-build

![alt text](https://i.ibb.co/z6qG5s7/progam-rev-02.png)

Go-118-fuzz-build is a tool to compile native Golang fuzzers to libFuzzer fuzzers. The tool was initially developed because continuous and CI fuzzing providers have developed platforms that depend on features in fuzzing engines that the native Go engine was not released with. To accomodate this, Go-118-fuzz-build changes the fuzz harnesses into libFuzzer harnesses that can then be intrumented with libFuzzer.

While it is not necessary to run Go-118-fuzz-build in a container, it is recommended, since it will change the source code of the project being built. See THIS_SECTION for an explanation on how it changes the project.

### Workflow

Say we have a native Go fuzz harness as such that we want to build as a libFuzzer fuzzer:
```go
package mypackage

import(
	"testing"
)

func FuzzMyApi(f *testing.F) {
	f.Fuzz(func(t *testing.T, name string, raw []byte) {
    	ApiOne(name)
        ApiTwo(raw)
	})
)
```

To do that, Go-118-fuzz-build does several things. 

First, it declares a libFuzzer entrypoint:

```go
// Code generated by go-118-fuzz-build; DO NOT EDIT.
// +build ignore
package main
import (
	"runtime"
	"strings"
	"unsafe"
	target "mypackage"
	"github.com/AdamKorcz/go-118-fuzz-build/testing"
)
// #include <stdint.h>
import "C"
//export LLVMFuzzerTestOneInput
func LLVMFuzzerTestOneInput(data *C.char, size C.size_t) C.int {
	s := (*[1<<30]byte)(unsafe.Pointer(data))[:size:size]
	defer catchPanics()
	LibFuzzerFuzzMyApi(s)
	return 0
}
func LibFuzzerFuzzMyApi(data []byte) int {
	fuzzer := &testing.F{Data:data, T:&testing.T{}}
	target.FuzzMyApi(fuzzer)
	return 1
}
func catchPanics() {
	if r := recover(); r != nil {
		var err string
		switch r.(type) {
		case string:
			err = r.(string)
		case runtime.Error:
			err = r.(runtime.Error).Error()
		case error:
			err = r.(error).Error()
		}
		if strings.Contains(err, "GO-FUZZ-BUILD-PANIC") {
			return
		} else {
			panic(err)
		}
	}
}
func main() {
}
```

You may notice that the libFuzzer harness creates a `github.com/AdamKorcz/go-118-fuzz-build/testing.F{}` which holds the data from the fuzzing engine. This is passed onto the native Go harness. But how does that work, since the native Go harness accepts a std lib `testing.F{}` type? Go-118-fuzz-build changes our target harness from this:

```go
package mypackage

import(
	"testing"
)

func FuzzMyApi(f *testing.F) {
	f.Fuzz(func(t *testing.T, name string, raw []byte) {
    	ApiOne(name)
        ApiTwo(raw)
	})
)
```

... to this:

```go
package mypackage

import(
	"github.com/AdamKorcz/go-118-fuzz-build/testing"
)

func FuzzMyApi(f *testing.F) {
	f.Fuzz(func(t *testing.T, name string, raw []byte) {
    	ApiOne(name)
        ApiTwo(raw)
	})
)
```

`github.com/AdamKorcz/go-118-fuzz-build/testing"` implements an `f.Fuzz()` that loops through all the parameters in the `f.Fuzz()` function (except the first which is alway a `testing.T` type. For each parameter, Go-118-fuzz-build will create a value of the specified type based on the libFuzzer input. For example, say we want a string and a []byte in our native Go harness, and the libFuzzer testcase is `0x03 0x41 0x42 0x43 0x03 0x44 0x45 0x46`: In that case our `string` will be `"ABC"` and our `[]byte` will be `[]byte(0x44, 0x45, 0x46)`. Go-118-fuzz-build uses [go-fuzz-headers](https://github.com/AdaLogics/go-fuzz-headers) to get the values in `f.Fuzz()`

You will notice that when we change `testing` to `github.com/AdamKorcz/go-118-fuzz-build/testing`, all uses of `testing`, for example `testing.T` and subsequently `t.Skip()`, `t.Error()` will come from the `github.com/AdamKorcz/go-118-fuzz-build/testing` library. As such, Go-118-fuzz-build implements a custom `testing` package that mimics the standard library testing package in a way that makes sense for libFuzzer. You could say that we translate all `testing.T` and `testing.F` methods in ways that make sense in a libFuzzer context. For example, currently, when you fuzz with a native Go harness, `t.Error()` will report an error after the whole fuzz run is over. This is not practical in a libFuzzer fuzz run, so instead we terminate immediately. 

## Goals of Go-118-fuzz-build
The fundamental high-level goal of Go-118-fuzz-build is to provide developers a similar experience when building and running their fuzzers using the native Go engine and libFuzzers engine. It should be as easy to build libFuzzer harnesses as it is to run `go test -c -fuzz=FuzzMyApi`. It is currently not as easy, for the reasons listed below.

## To-Dos

Go-118-fuzz-build does not handle the following cases which causes the build process to not be as seamless as is the goal:

- `_test.go` files are currently not included in the scope of the build. These will have to be manually changes to not end in `_test.go`.
- Helper functions in separate files will not be modified to accept `github.com/AdamKorcz/go-118-fuzz-build/testing`.T type instead of `testing.T`.
- The `github.com/AdamKorcz/go-118-fuzz-build/testing` dependency needs to manually be added to the `go.mod` file. This is not always easy, especially with projects using vendoring.

## Dependencies
Currently, to use this tool, you *must* have `github.com/AdamKorcz/go-118-fuzz-build/testing` in your `go.mod`. The easiest way to do this independently of whether you use vendoring is to create a dummy file that imports `github.com/AdamKorcz/go-118-fuzz-build/testing` somewhere in your project. This can be done with this line:

```bash
printf "package main\nimport _ \"github.com/AdamKorcz/go-118-fuzz-build/testing\"\n" > register.go
```

When you then run `go mod tidy`, Go will handle the dependencies.

## How to use

Here we enumerate common ways to use go-118-fuzz-build.

#### Preparation
Any usage requires the go-118-fuzz-build binary:

```bash
git clone https://github.com/AdamKorcz/go-118-fuzz-build
cd go-118-fuzz-build
go build
mv go-118-fuzz-build $GOPATH/bin/ # or add to $PATH instead
```

With that, let's discover some ways to use the tool:

#### The easy
A simple fuzzer can be built like this:

```bash
git clone https://github.com/my/project
cd project/package1
printf "package package1\nimport _ \"github.com/AdamKorcz/go-118-fuzz-build/testing\"\n" > registerfuzzdependency.go
go-118-fuzz-build -o fuzz_archive_file.a -func FuzzMyApi github.com/my/project
clang -o fuzz_binary fuzz_archive_file.a -fsanitize=fuzzer
```

### Using test utils from other `*_test.go` files
go-118-fuzz-build cannot read any `*_test.go` files. These will need to be renamed so they don't end in `_test.go`.
In this example our fuzzer uses utilities from `utils_test.go`, so we rename that file to `utils_test_fuzz.go`.
```bash
git clone https://github.com/my/project
cd project/package1
printf "package package1\nimport _ \"github.com/AdamKorcz/go-118-fuzz-build/testing\"\n" > registerfuzzdependency.go
mv utils_test.go utils_test_fuzz.go # has some functions that we use in our fuzzer.
go-118-fuzz-build -o fuzz_archive_file.a -func FuzzMyApi github.com/my/project
clang -o fuzz_binary fuzz_archive_file.a -fsanitize=fuzzer
```
