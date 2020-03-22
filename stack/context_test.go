// Copyright 2018 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package stack

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/maruel/panicparse/internal/internaltest"
)

func TestParseDumpNothing(t *testing.T) {
	extra := &bytes.Buffer{}
	c, err := ParseDump(bytes.NewBufferString("\n"), extra, true)
	if err != nil {
		t.Fatal(err)
	}
	if c != nil {
		t.Fatalf("unexpected %v", c)
	}
}

func TestParseDump1(t *testing.T) {
	// One call from main, one from stdlib, one from third party.
	// Create a long first line that will be ignored. It is to guard against
	// https://github.com/maruel/panicparse/issues/17.
	long := strings.Repeat("a", bufio.MaxScanTokenSize+1)
	data := []string{
		long,
		"panic: reflect.Set: value of type",
		"",
		"goroutine 1 [running]:",
		"github.com/cockroachdb/cockroach/storage/engine._Cfunc_DBIterSeek()",
		" ??:0 +0x6d",
		"gopkg.in/yaml%2ev2.handleErr(0xc208033b20)",
		"	/gopath/src/gopkg.in/yaml.v2/yaml.go:153 +0xc6",
		"reflect.Value.assignTo(0x570860, 0xc20803f3e0, 0x15)",
		"	/goroot/src/reflect/value.go:2125 +0x368",
		"main.main()",
		"	/gopath/src/github.com/maruel/panicparse/stack/stack.go:428 +0x27",
		"",
	}
	extra := &bytes.Buffer{}
	c, err := ParseDump(bytes.NewBufferString(strings.Join(data, "\n")), extra, true)
	if err != nil {
		t.Fatal(err)
	}
	compareString(t, long+"\npanic: reflect.Set: value of type\n\n", extra.String())
	expected := []*Goroutine{
		{
			Signature: Signature{
				State: "running",
				Stack: Stack{
					Calls: []Call{
						{
							SrcPath: "??",
							Func:    Func{Raw: "github.com/cockroachdb/cockroach/storage/engine._Cfunc_DBIterSeek"},
						},
						{
							SrcPath: "/gopath/src/gopkg.in/yaml.v2/yaml.go",
							Line:    153,
							Func:    Func{Raw: "gopkg.in/yaml%2ev2.handleErr"},
							Args:    Args{Values: []Arg{{Value: 0xc208033b20}}},
						},
						{
							SrcPath: "/goroot/src/reflect/value.go",
							Line:    2125,
							Func:    Func{Raw: "reflect.Value.assignTo"},
							Args:    Args{Values: []Arg{{Value: 0x570860}, {Value: 0xc20803f3e0}, {Value: 0x15}}},
						},
						{
							SrcPath: "/gopath/src/github.com/maruel/panicparse/stack/stack.go",
							Line:    428,
							Func:    Func{Raw: "main.main"},
						},
					},
				},
			},
			ID:    1,
			First: true,
		},
	}
	for i := range expected {
		expected[i].updateLocations(c.GOROOT, c.localgoroot, c.GOPATHs)
	}
	compareGoroutines(t, expected, c.Goroutines)
}

func TestParseDumpLongWait(t *testing.T) {
	// One call from main, one from stdlib, one from third party.
	data := []string{
		"panic: bleh",
		"",
		"goroutine 1 [chan send, 100 minutes]:",
		"gopkg.in/yaml%2ev2.handleErr(0xc208033b20)",
		"	/gopath/src/gopkg.in/yaml.v2/yaml.go:153 +0xc6",
		"",
		"goroutine 2 [chan send, locked to thread]:",
		"gopkg.in/yaml%2ev2.handleErr(0xc208033b21)",
		"	/gopath/src/gopkg.in/yaml.v2/yaml.go:153 +0xc6",
		"",
		"goroutine 3 [chan send, 101 minutes, locked to thread]:",
		"gopkg.in/yaml%2ev2.handleErr(0xc208033b22)",
		"	/gopath/src/gopkg.in/yaml.v2/yaml.go:153 +0xc6",
		"",
	}
	extra := &bytes.Buffer{}
	c, err := ParseDump(bytes.NewBufferString(strings.Join(data, "\n")), extra, true)
	if err != nil {
		t.Fatal(err)
	}
	compareString(t, "panic: bleh\n\n", extra.String())
	expected := []*Goroutine{
		{
			Signature: Signature{
				State:    "chan send",
				SleepMin: 100,
				SleepMax: 100,
				Stack: Stack{
					Calls: []Call{
						{
							SrcPath: "/gopath/src/gopkg.in/yaml.v2/yaml.go",
							Line:    153,
							Func:    Func{Raw: "gopkg.in/yaml%2ev2.handleErr"},
							Args:    Args{Values: []Arg{{Value: 0xc208033b20}}},
						},
					},
				},
			},
			ID:    1,
			First: true,
		},
		{
			Signature: Signature{
				State:  "chan send",
				Locked: true,
				Stack: Stack{
					Calls: []Call{
						{
							SrcPath: "/gopath/src/gopkg.in/yaml.v2/yaml.go",
							Line:    153,
							Func:    Func{Raw: "gopkg.in/yaml%2ev2.handleErr"},
							Args:    Args{Values: []Arg{{Value: 0xc208033b21, Name: "#1"}}},
						},
					},
				},
			},
			ID: 2,
		},
		{
			Signature: Signature{
				State:    "chan send",
				SleepMin: 101,
				SleepMax: 101,
				Stack: Stack{
					Calls: []Call{
						{
							SrcPath: "/gopath/src/gopkg.in/yaml.v2/yaml.go",
							Line:    153,
							Func:    Func{Raw: "gopkg.in/yaml%2ev2.handleErr"},
							Args:    Args{Values: []Arg{{Value: 0xc208033b22, Name: "#2"}}},
						},
					},
				},
				Locked: true,
			},
			ID: 3,
		},
	}
	for i := range expected {
		expected[i].updateLocations(c.GOROOT, c.localgoroot, c.GOPATHs)
	}
	compareGoroutines(t, expected, c.Goroutines)
}

func TestParseDumpAsm(t *testing.T) {
	data := []string{
		"panic: reflect.Set: value of type",
		"",
		"goroutine 16 [garbage collection]:",
		"runtime.switchtoM()",
		"\t/goroot/src/runtime/asm_amd64.s:198 fp=0xc20cfb80d8 sp=0xc20cfb80d0",
		"",
	}
	extra := &bytes.Buffer{}
	c, err := ParseDump(bytes.NewBufferString(strings.Join(data, "\n")), extra, false)
	if err != nil {
		t.Fatal(err)
	}
	expected := []*Goroutine{
		{
			Signature: Signature{
				State: "garbage collection",
				Stack: Stack{
					Calls: []Call{
						{
							SrcPath: "/goroot/src/runtime/asm_amd64.s",
							Line:    198,
							Func:    Func{Raw: "runtime.switchtoM"},
						},
					},
				},
			},
			ID:    16,
			First: true,
		},
	}
	compareGoroutines(t, expected, c.Goroutines)
	compareString(t, "panic: reflect.Set: value of type\n\n", extra.String())
}

func TestParseDumpAsmGo1dot13(t *testing.T) {
	data := []string{
		"panic: reflect.Set: value of type",
		"",
		"goroutine 16 [garbage collection]:",
		"runtime.switchtoM()",
		"\t/goroot/src/runtime/asm_amd64.s:198 fp=0xc20cfb80d8 sp=0xc20cfb80d0 pc=0x5007be",
		"",
	}
	extra := &bytes.Buffer{}
	c, err := ParseDump(bytes.NewBufferString(strings.Join(data, "\n")), extra, false)
	if err != nil {
		t.Fatal(err)
	}
	expected := []*Goroutine{
		{
			Signature: Signature{
				State: "garbage collection",
				Stack: Stack{
					Calls: []Call{
						{
							SrcPath: "/goroot/src/runtime/asm_amd64.s",
							Line:    198,
							Func:    Func{Raw: "runtime.switchtoM"},
						},
					},
				},
			},
			ID:    16,
			First: true,
		},
	}
	compareGoroutines(t, expected, c.Goroutines)
	compareString(t, "panic: reflect.Set: value of type\n\n", extra.String())
}

func TestParseDumpLineErr(t *testing.T) {
	data := []string{
		"panic: reflect.Set: value of type",
		"",
		"goroutine 1 [running]:",
		"github.com/maruel/panicparse/stack/stack.recurseType()",
		"\t/gopath/src/github.com/maruel/panicparse/stack/stack.go:12345678901234567890",
		"",
	}
	extra := &bytes.Buffer{}
	c, err := ParseDump(bytes.NewBufferString(strings.Join(data, "\n")), extra, false)
	compareErr(t, errors.New("failed to parse int on line: \"/gopath/src/github.com/maruel/panicparse/stack/stack.go:12345678901234567890\""), err)
	expected := []*Goroutine{
		{
			Signature: Signature{
				State: "running",
				Stack: Stack{Calls: []Call{{Func: Func{Raw: "github.com/maruel/panicparse/stack/stack.recurseType"}}}},
			},
			ID:    1,
			First: true,
		},
	}
	for i := range expected {
		expected[i].updateLocations(c.GOROOT, c.localgoroot, c.GOPATHs)
	}
	compareGoroutines(t, expected, c.Goroutines)
}

func TestParseDumpCreatedErr(t *testing.T) {
	data := []string{
		"panic: reflect.Set: value of type",
		"",
		"goroutine 1 [running]:",
		"github.com/maruel/panicparse/stack/stack.recurseType()",
		"\t/gopath/src/github.com/maruel/panicparse/stack/stack.go:1",
		"created by testing.RunTests",
		"\t/goroot/src/testing/testing.go:123456789012345678901 +0xa8b",
		"",
	}
	extra := &bytes.Buffer{}
	c, err := ParseDump(bytes.NewBufferString(strings.Join(data, "\n")), extra, false)
	compareErr(t, errors.New("failed to parse int on line: \"/goroot/src/testing/testing.go:123456789012345678901 +0xa8b\""), err)
	expected := []*Goroutine{
		{
			Signature: Signature{
				State: "running",
				Stack: Stack{
					Calls: []Call{
						{
							SrcPath: "/gopath/src/github.com/maruel/panicparse/stack/stack.go",
							Line:    1,
							Func:    Func{Raw: "github.com/maruel/panicparse/stack/stack.recurseType"},
						},
					},
				},
				CreatedBy: Call{
					Func: Func{Raw: "testing.RunTests"},
				},
			},
			ID:    1,
			First: true,
		},
	}
	compareGoroutines(t, expected, c.Goroutines)
}

func TestParseDumpValueErr(t *testing.T) {
	data := []string{
		"panic: reflect.Set: value of type",
		"",
		"goroutine 1 [running]:",
		"github.com/maruel/panicparse/stack/stack.recurseType(123456789012345678901)",
		"\t/gopath/src/github.com/maruel/panicparse/stack/stack.go:9",
		"",
	}
	extra := &bytes.Buffer{}
	c, err := ParseDump(bytes.NewBufferString(strings.Join(data, "\n")), extra, false)
	compareErr(t, errors.New("failed to parse int on line: \"github.com/maruel/panicparse/stack/stack.recurseType(123456789012345678901)\""), err)
	expected := []*Goroutine{
		{
			Signature: Signature{
				State: "running",
				Stack: Stack{
					Calls: []Call{{Func: Func{Raw: "github.com/maruel/panicparse/stack/stack.recurseType"}}},
				},
			},
			ID:    1,
			First: true,
		},
	}
	for i := range expected {
		expected[i].updateLocations(c.GOROOT, c.localgoroot, c.GOPATHs)
	}
	compareGoroutines(t, expected, c.Goroutines)
}

func TestParseDumpInconsistentIndent(t *testing.T) {
	data := []string{
		"  goroutine 1 [running]:",
		"  github.com/maruel/panicparse/stack/stack.recurseType()",
		" \t/gopath/src/github.com/maruel/panicparse/stack/stack.go:1",
		"",
	}
	extra := &bytes.Buffer{}
	c, err := ParseDump(bytes.NewBufferString(strings.Join(data, "\n")), extra, false)
	compareErr(t, errors.New(`inconsistent indentation: " \t/gopath/src/github.com/maruel/panicparse/stack/stack.go:1", expected "  "`), err)
	expected := []*Goroutine{
		{
			Signature: Signature{
				State: "running",
				Stack: Stack{
					Calls: []Call{
						{Func: Func{Raw: "github.com/maruel/panicparse/stack/stack.recurseType"}},
					},
				},
			},
			ID:    1,
			First: true,
		},
	}
	compareGoroutines(t, expected, c.Goroutines)
	compareString(t, "", extra.String())
}

func TestParseDumpOrderErr(t *testing.T) {
	data := []string{
		"panic: reflect.Set: value of type",
		"",
		"goroutine 16 [garbage collection]:",
		"	/gopath/src/gopkg.in/yaml.v2/yaml.go:153 +0xc6",
		"runtime.switchtoM()",
		"\t/goroot/src/runtime/asm_amd64.s:198 fp=0xc20cfb80d8 sp=0xc20cfb80d0",
		"",
	}
	extra := &bytes.Buffer{}
	c, err := ParseDump(bytes.NewBufferString(strings.Join(data, "\n")), extra, false)
	compareErr(t, errors.New("expected a function after a goroutine header, got: \"/gopath/src/gopkg.in/yaml.v2/yaml.go:153 +0xc6\""), err)
	expected := []*Goroutine{
		{
			Signature: Signature{State: "garbage collection"},
			ID:        16,
			First:     true,
		},
	}
	compareGoroutines(t, expected, c.Goroutines)
	compareString(t, "panic: reflect.Set: value of type\n\n", extra.String())
}

func TestParseDumpElided(t *testing.T) {
	data := []string{
		"panic: reflect.Set: value of type",
		"",
		"goroutine 16 [garbage collection]:",
		"github.com/maruel/panicparse/stack/stack.recurseType(0x7f4fa9a3ec70, 0xc208062580, 0x7f4fa9a3e818, 0x50a820, 0xc20803a8a0)",
		"\t/gopath/src/github.com/maruel/panicparse/stack/stack.go:53 +0x845 fp=0xc20cfc66d8 sp=0xc20cfc6470",
		"...additional frames elided...",
		"created by testing.RunTests",
		"\t/goroot/src/testing/testing.go:555 +0xa8b",
		"",
	}
	extra := &bytes.Buffer{}
	c, err := ParseDump(bytes.NewBufferString(strings.Join(data, "\n")), extra, false)
	if err != nil {
		t.Fatal(err)
	}
	expected := []*Goroutine{
		{
			Signature: Signature{
				State: "garbage collection",
				Stack: Stack{
					Calls: []Call{
						{
							SrcPath: "/gopath/src/github.com/maruel/panicparse/stack/stack.go",
							Line:    53,
							Func:    Func{Raw: "github.com/maruel/panicparse/stack/stack.recurseType"},
							Args: Args{
								Values: []Arg{
									{Value: 0x7f4fa9a3ec70},
									{Value: 0xc208062580},
									{Value: 0x7f4fa9a3e818},
									{Value: 0x50a820},
									{Value: 0xc20803a8a0},
								},
							},
						},
					},
					Elided: true,
				},
				CreatedBy: Call{
					SrcPath: "/goroot/src/testing/testing.go",
					Line:    555,
					Func:    Func{Raw: "testing.RunTests"},
				},
			},
			ID:    16,
			First: true,
		},
	}
	compareGoroutines(t, expected, c.Goroutines)
	compareString(t, "panic: reflect.Set: value of type\n\n", extra.String())
}

func TestParseDumpSysCall(t *testing.T) {
	data := []string{
		"panic: reflect.Set: value of type",
		"",
		"goroutine 5 [syscall]:",
		"runtime.notetsleepg(0x918100, 0xffffffffffffffff, 0x1)",
		"\t/goroot/src/runtime/lock_futex.go:201 +0x52 fp=0xc208018f68 sp=0xc208018f40",
		"runtime.signal_recv(0x0)",
		"\t/goroot/src/runtime/sigqueue.go:109 +0x135 fp=0xc208018fa0 sp=0xc208018f68",
		"os/signal.loop()",
		"\t/goroot/src/os/signal/signal_unix.go:21 +0x1f fp=0xc208018fe0 sp=0xc208018fa0",
		"runtime.goexit()",
		"\t/goroot/src/runtime/asm_amd64.s:2232 +0x1 fp=0xc208018fe8 sp=0xc208018fe0",
		"created by os/signal.init·1",
		"\t/goroot/src/os/signal/signal_unix.go:27 +0x35",
		"",
	}
	extra := &bytes.Buffer{}
	c, err := ParseDump(bytes.NewBufferString(strings.Join(data, "\n")), extra, false)
	if err != nil {
		t.Fatal(err)
	}
	expected := []*Goroutine{
		{
			Signature: Signature{
				State: "syscall",
				Stack: Stack{
					Calls: []Call{
						{
							SrcPath: "/goroot/src/runtime/lock_futex.go",
							Line:    201,
							Func:    Func{Raw: "runtime.notetsleepg"},
							Args: Args{
								Values: []Arg{
									{Value: 0x918100},
									{Value: 0xffffffffffffffff},
									{Value: 0x1},
								},
							},
						},
						{
							SrcPath: "/goroot/src/runtime/sigqueue.go",
							Line:    109,
							Func:    Func{Raw: "runtime.signal_recv"},
							Args: Args{
								Values: []Arg{{}},
							},
						},
						{
							SrcPath: "/goroot/src/os/signal/signal_unix.go",
							Line:    21,
							Func:    Func{Raw: "os/signal.loop"},
						},
						{
							SrcPath: "/goroot/src/runtime/asm_amd64.s",
							Line:    2232,
							Func:    Func{Raw: "runtime.goexit"},
						},
					},
				},
				CreatedBy: Call{
					SrcPath: "/goroot/src/os/signal/signal_unix.go",
					Line:    27,
					Func:    Func{Raw: "os/signal.init·1"},
				},
			},
			ID:    5,
			First: true,
		},
	}
	compareGoroutines(t, expected, c.Goroutines)
	compareString(t, "panic: reflect.Set: value of type\n\n", extra.String())
}

func TestParseDumpUnavailCreated(t *testing.T) {
	data := []string{
		"panic: reflect.Set: value of type",
		"",
		"goroutine 24 [running]:",
		"\tgoroutine running on other thread; stack unavailable",
		"created by github.com/maruel/panicparse/stack.New",
		"\t/gopath/src/github.com/maruel/panicparse/stack/stack.go:131 +0x381",
		"",
	}
	extra := &bytes.Buffer{}
	c, err := ParseDump(bytes.NewBufferString(strings.Join(data, "\n")), extra, false)
	if err != nil {
		t.Fatal(err)
	}
	expected := []*Goroutine{
		{
			Signature: Signature{
				State: "running",
				Stack: Stack{
					Calls: []Call{{SrcPath: "<unavailable>"}},
				},
				CreatedBy: Call{
					SrcPath: "/gopath/src/github.com/maruel/panicparse/stack/stack.go",
					Line:    131,
					Func:    Func{Raw: "github.com/maruel/panicparse/stack.New"},
				},
			},
			ID:    24,
			First: true,
		},
	}
	compareGoroutines(t, expected, c.Goroutines)
	compareString(t, "panic: reflect.Set: value of type\n\n", extra.String())
}

func TestParseDumpUnavail(t *testing.T) {
	data := []string{
		"panic: reflect.Set: value of type",
		"",
		"goroutine 24 [running]:",
		"\tgoroutine running on other thread; stack unavailable",
		"",
		"",
	}
	extra := &bytes.Buffer{}
	c, err := ParseDump(bytes.NewBufferString(strings.Join(data, "\n")), extra, false)
	if err != nil {
		t.Fatal(err)
	}
	expected := []*Goroutine{
		{
			Signature: Signature{
				State: "running",
				Stack: Stack{
					Calls: []Call{{SrcPath: "<unavailable>"}},
				},
			},
			ID:    24,
			First: true,
		},
	}
	compareGoroutines(t, expected, c.Goroutines)
	compareString(t, "panic: reflect.Set: value of type\n\n", extra.String())
}

func TestParseDumpUnavailError(t *testing.T) {
	data := []string{
		"panic: reflect.Set: value of type",
		"",
		"goroutine 24 [running]:",
		"\tgoroutine running on other thread; stack unavailable",
		"junk",
	}
	extra := &bytes.Buffer{}
	c, err := ParseDump(bytes.NewBufferString(strings.Join(data, "\n")), extra, false)
	compareErr(t, errors.New("expected empty line after unavailable stack, got: \"junk\""), err)
	expected := []*Goroutine{
		{
			Signature: Signature{
				State: "running",
				Stack: Stack{
					Calls: []Call{{SrcPath: "<unavailable>"}},
				},
			},
			ID:    24,
			First: true,
		},
	}
	compareGoroutines(t, expected, c.Goroutines)
	compareString(t, "panic: reflect.Set: value of type\n\n", extra.String())
}

func TestParseDumpNoOffset(t *testing.T) {
	data := []string{
		"panic: runtime error: index out of range",
		"",
		"goroutine 37 [runnable]:",
		"github.com/maruel/panicparse/stack.func·002()",
		"	/gopath/src/github.com/maruel/panicparse/stack/stack.go:110",
		"created by github.com/maruel/panicparse/stack.New",
		"	/gopath/src/github.com/maruel/panicparse/stack/stack.go:113 +0x43b",
		"",
	}
	c, err := ParseDump(bytes.NewBufferString(strings.Join(data, "\n")), ioutil.Discard, false)
	if err != nil {
		t.Fatal(err)
	}
	expectedGR := []*Goroutine{
		{
			Signature: Signature{
				State: "runnable",
				Stack: Stack{
					Calls: []Call{
						{
							SrcPath: "/gopath/src/github.com/maruel/panicparse/stack/stack.go",
							Line:    110,
							Func:    Func{Raw: "github.com/maruel/panicparse/stack.func·002"},
						},
					},
				},
				CreatedBy: Call{
					SrcPath: "/gopath/src/github.com/maruel/panicparse/stack/stack.go",
					Line:    113,
					Func:    Func{Raw: "github.com/maruel/panicparse/stack.New"},
				},
			},
			ID:    37,
			First: true,
		},
	}
	compareGoroutines(t, expectedGR, c.Goroutines)
}

func TestParseDumpHeaderError(t *testing.T) {
	// For coverage of scanLines.
	data := []string{
		"panic: reflect.Set: value of type",
		"",
		"goroutine 1 [running]:",
		"junk",
	}
	extra := &bytes.Buffer{}
	c, err := ParseDump(bytes.NewBufferString(strings.Join(data, "\n")), extra, false)
	compareErr(t, errors.New("expected a function after a goroutine header, got: \"junk\""), err)
	expected := []*Goroutine{
		{
			Signature: Signature{State: "running"},
			ID:        1,
			First:     true,
		},
	}
	compareGoroutines(t, expected, c.Goroutines)
	compareString(t, "panic: reflect.Set: value of type\n\n", extra.String())
}

func TestParseDumpFileError(t *testing.T) {
	// For coverage of scanLines.
	data := []string{
		"panic: reflect.Set: value of type",
		"",
		"goroutine 1 [running]:",
		"github.com/maruel/panicparse/stack.func·002()",
		"junk",
	}
	extra := &bytes.Buffer{}
	c, err := ParseDump(bytes.NewBufferString(strings.Join(data, "\n")), extra, false)
	compareErr(t, errors.New("expected a file after a function, got: \"junk\""), err)
	expected := []*Goroutine{
		{
			Signature: Signature{
				State: "running",
				Stack: Stack{
					Calls: []Call{
						{Func: Func{Raw: "github.com/maruel/panicparse/stack.func·002"}},
					},
				},
			},
			ID:    1,
			First: true,
		},
	}
	compareGoroutines(t, expected, c.Goroutines)
	compareString(t, "panic: reflect.Set: value of type\n\n", extra.String())
}

func TestParseDumpCreated(t *testing.T) {
	// For coverage of scanLines.
	data := []string{
		"panic: reflect.Set: value of type",
		"",
		"goroutine 1 [running]:",
		"github.com/maruel/panicparse/stack.func·002()",
		"	/gopath/src/github.com/maruel/panicparse/stack/stack.go:110",
		"created by github.com/maruel/panicparse/stack.New",
		"\t/gopath/src/github.com/maruel/panicparse/stack/stack.go:131 +0x381",
		"exit status 2",
	}
	extra := &bytes.Buffer{}
	c, err := ParseDump(bytes.NewBufferString(strings.Join(data, "\n")), extra, false)
	if err != nil {
		t.Fatal(err)
	}
	expected := []*Goroutine{
		{
			Signature: Signature{
				State: "running",
				Stack: Stack{
					Calls: []Call{
						{
							SrcPath: "/gopath/src/github.com/maruel/panicparse/stack/stack.go",
							Line:    110,
							Func:    Func{Raw: "github.com/maruel/panicparse/stack.func·002"},
						},
					},
				},
				CreatedBy: Call{
					SrcPath: "/gopath/src/github.com/maruel/panicparse/stack/stack.go",
					Line:    131,
					Func:    Func{Raw: "github.com/maruel/panicparse/stack.New"},
				},
			},
			ID:    1,
			First: true,
		},
	}
	compareGoroutines(t, expected, c.Goroutines)
	compareString(t, "panic: reflect.Set: value of type\n\nexit status 2", extra.String())
}

func TestParseDumpCreatedError(t *testing.T) {
	// For coverage of scanLines.
	data := []string{
		"panic: reflect.Set: value of type",
		"",
		"goroutine 1 [running]:",
		"github.com/maruel/panicparse/stack.func·002()",
		"	/gopath/src/github.com/maruel/panicparse/stack/stack.go:110",
		"created by github.com/maruel/panicparse/stack.New",
		"junk",
	}
	extra := &bytes.Buffer{}
	c, err := ParseDump(bytes.NewBufferString(strings.Join(data, "\n")), extra, false)
	compareErr(t, errors.New("expected a file after a created line, got: \"junk\""), err)
	expected := []*Goroutine{
		{
			Signature: Signature{
				State: "running",
				Stack: Stack{
					Calls: []Call{
						{
							SrcPath: "/gopath/src/github.com/maruel/panicparse/stack/stack.go",
							Line:    110,
							Func:    Func{Raw: "github.com/maruel/panicparse/stack.func·002"},
						},
					},
				},
				CreatedBy: Call{
					Func: Func{Raw: "github.com/maruel/panicparse/stack.New"},
				},
			},
			ID:    1,
			First: true,
		},
	}
	compareGoroutines(t, expected, c.Goroutines)
	compareString(t, "panic: reflect.Set: value of type\n\n", extra.String())
}

func TestParseDumpCCode(t *testing.T) {
	data := []string{
		"SIGQUIT: quit",
		"PC=0x43f349",
		"",
		"goroutine 0 [idle]:",
		"runtime.epollwait(0x4, 0x7fff671c7118, 0xffffffff00000080, 0x0, 0xffffffff0028c1be, 0x0, 0x0, 0x0, 0x0, 0x0, ...)",
		"        /goroot/src/runtime/sys_linux_amd64.s:400 +0x19",
		"runtime.netpoll(0x901b01, 0x0)",
		"        /goroot/src/runtime/netpoll_epoll.go:68 +0xa3",
		"findrunnable(0xc208012000)",
		"        /goroot/src/runtime/proc.c:1472 +0x485",
		"schedule()",
		"        /goroot/src/runtime/proc.c:1575 +0x151",
		"runtime.park_m(0xc2080017a0)",
		"        /goroot/src/runtime/proc.c:1654 +0x113",
		"runtime.mcall(0x432684)",
		"        /goroot/src/runtime/asm_amd64.s:186 +0x5a",
		"",
	}
	c, err := ParseDump(bytes.NewBufferString(strings.Join(data, "\n")), ioutil.Discard, false)
	if err != nil {
		t.Fatal(err)
	}
	expectedGR := []*Goroutine{
		{
			Signature: Signature{
				State: "idle",
				Stack: Stack{
					Calls: []Call{
						{
							SrcPath: "/goroot/src/runtime/sys_linux_amd64.s",
							Line:    400,
							Func:    Func{Raw: "runtime.epollwait"},
							Args: Args{
								Values: []Arg{
									{Value: 0x4},
									{Value: 0x7fff671c7118},
									{Value: 0xffffffff00000080},
									{},
									{Value: 0xffffffff0028c1be},
									{},
									{},
									{},
									{},
									{},
								},
								Elided: true,
							},
						},
						{
							SrcPath: "/goroot/src/runtime/netpoll_epoll.go",
							Line:    68,
							Func:    Func{Raw: "runtime.netpoll"},
							Args:    Args{Values: []Arg{{Value: 0x901b01}, {}}},
						},
						{
							SrcPath: "/goroot/src/runtime/proc.c",
							Line:    1472,
							Func:    Func{Raw: "findrunnable"},
							Args:    Args{Values: []Arg{{Value: 0xc208012000}}},
						},
						{
							SrcPath: "/goroot/src/runtime/proc.c",
							Line:    1575,
							Func:    Func{Raw: "schedule"},
						},
						{
							SrcPath: "/goroot/src/runtime/proc.c",
							Line:    1654,
							Func:    Func{Raw: "runtime.park_m"},
							Args:    Args{Values: []Arg{{Value: 0xc2080017a0}}},
						},
						{
							SrcPath: "/goroot/src/runtime/asm_amd64.s",
							Line:    186,
							Func:    Func{Raw: "runtime.mcall"},
							Args:    Args{Values: []Arg{{Value: 0x432684}}},
						},
					},
				},
			},
			ID:    0,
			First: true,
		},
	}
	compareGoroutines(t, expectedGR, c.Goroutines)
}

func TestParseDumpWithCarriageReturn(t *testing.T) {
	data := []string{
		"goroutine 1 [running]:",
		"github.com/cockroachdb/cockroach/storage/engine._Cfunc_DBIterSeek()",
		" ??:0 +0x6d",
		"gopkg.in/yaml%2ev2.handleErr(0xc208033b20)",
		"	/gopath/src/gopkg.in/yaml.v2/yaml.go:153 +0xc6",
		"reflect.Value.assignTo(0x570860, 0xc20803f3e0, 0x15)",
		"	/goroot/src/reflect/value.go:2125 +0x368",
		"main.main()",
		"	/gopath/src/github.com/maruel/panicparse/stack/stack.go:428 +0x27",
		"",
	}

	c, err := ParseDump(bytes.NewBufferString(strings.Join(data, "\r\n")), ioutil.Discard, false)
	if err != nil {
		t.Fatal(err)
	}
	expected := []*Goroutine{
		{
			Signature: Signature{
				State: "running",
				Stack: Stack{
					Calls: []Call{
						{
							SrcPath: "??",
							Func:    Func{Raw: "github.com/cockroachdb/cockroach/storage/engine._Cfunc_DBIterSeek"},
						},
						{
							SrcPath: "/gopath/src/gopkg.in/yaml.v2/yaml.go",
							Line:    153,
							Func:    Func{Raw: "gopkg.in/yaml%2ev2.handleErr"},
							Args:    Args{Values: []Arg{{Value: 0xc208033b20}}},
						},
						{
							SrcPath: "/goroot/src/reflect/value.go",
							Line:    2125,
							Func:    Func{Raw: "reflect.Value.assignTo"},
							Args:    Args{Values: []Arg{{Value: 0x570860}, {Value: 0xc20803f3e0}, {Value: 0x15}}},
						},
						{
							SrcPath: "/gopath/src/github.com/maruel/panicparse/stack/stack.go",
							Line:    428,
							Func:    Func{Raw: "main.main"},
						},
					},
				},
			},
			ID:    1,
			First: true,
		},
	}
	compareGoroutines(t, expected, c.Goroutines)
}

func TestParseDumpIndented(t *testing.T) {
	// goconvey is culprit of this.
	data := []string{
		"Failures:",
		"",
		"  * /home/maruel/go/src/foo/bar_test.go",
		"  Line 209:",
		"  Expected: '(*errors.errorString){s:\"context canceled\"}'",
		"  Actual:   'nil'",
		"  (Should resemble)!",
		"  goroutine 8 [running]:",
		"  foo/bar.TestArchiveFail.func1.2()",
		"        /home/maruel/go/foo/bar_test.go:209 +0x469",
		"  foo/bar.TestArchiveFail(0xc000338200)",
		"        /home/maruel/go/src/foo/bar_test.go:155 +0xf1",
		"  testing.tRunner(0xc000338200, 0x1615bf8)",
		"        /home/maruel/golang/go/src/testing/testing.go:865 +0xc0",
		"  created by testing.(*T).Run",
		"        /home/maruel/golang/go/src/testing/testing.go:916 +0x35a",
		"",
		"",
	}
	extra := bytes.Buffer{}
	c, err := ParseDump(bytes.NewBufferString(strings.Join(data, "\n")), &extra, false)
	if err != nil {
		t.Fatal(err)
	}
	compareString(t, strings.Join(data[:7], "\n")+"\n", extra.String())
	expected := []*Goroutine{
		{
			Signature: Signature{
				State: "running",
				Stack: Stack{
					Calls: []Call{
						{
							SrcPath: "/home/maruel/go/foo/bar_test.go",
							Line:    209,
							Func:    Func{Raw: "foo/bar.TestArchiveFail.func1.2"},
						},
						{
							SrcPath: "/home/maruel/go/src/foo/bar_test.go",
							Line:    155,
							Func:    Func{Raw: "foo/bar.TestArchiveFail"},
							Args:    Args{Values: []Arg{{Value: 0xc000338200, Name: "#1"}}},
						},
						{
							SrcPath: "/home/maruel/golang/go/src/testing/testing.go",
							Line:    865,
							Func:    Func{Raw: "testing.tRunner"},
							Args:    Args{Values: []Arg{{Value: 0xc000338200, Name: "#1"}, {Value: 0x1615bf8}}},
						},
					},
				},
				CreatedBy: Call{
					SrcPath: "/home/maruel/golang/go/src/testing/testing.go",
					Line:    916,
					Func:    Func{Raw: "testing.(*T).Run"},
				},
			},
			ID:    8,
			First: true,
		},
	}
	compareGoroutines(t, expected, c.Goroutines)
}

func TestParseDumpRace(t *testing.T) {
	// Generated with "panic race":
	data := []string{
		"==================",
		"WARNING: DATA RACE",
		"Read at 0x00c0000e4030 by goroutine 7:",
		"  main.panicRace.func1()",
		"      /go/src/github.com/maruel/panicparse/cmd/panic/main_race.go:37 +0x38",
		"",
		"Previous write at 0x00c0000e4030 by goroutine 6:",
		"  main.panicRace.func1()",
		"      /go/src/github.com/maruel/panicparse/cmd/panic/main_race.go:37 +0x4e",
		"",
		"Goroutine 7 (running) created at:",
		"  main.panicRace()",
		"      /go/src/github.com/maruel/panicparse/cmd/panic/main_race.go:35 +0x88",
		"  main.main()",
		"      /go/src/github.com/maruel/panicparse/cmd/panic/main.go:252 +0x2d9",
		"",
		"Goroutine 6 (running) created at:",
		"  main.panicRace()",
		"      /go/src/github.com/maruel/panicparse/cmd/panic/main_race.go:35 +0x88",
		"  main.main()",
		"      /go/src/github.com/maruel/panicparse/cmd/panic/main.go:252 +0x2d9",
		"==================",
		"",
	}
	extra := &bytes.Buffer{}
	c, err := ParseDump(bytes.NewBufferString(strings.Join(data, "\n")), extra, false)
	if err != nil {
		t.Fatal(err)
	}
	// Confirm that it doesn't work yet.
	if c != nil {
		t.Fatal("expected c to be nil")
	}
	compareString(t, strings.Join(data, "\n"), extra.String())
}

// This test should be deleted once Context state.raceDetectionEnabled is
// removed and the race detector results is stored in Context.
func TestRaceManual(t *testing.T) {
	// Generated with "panic race":
	data := []string{
		"==================",
		"WARNING: DATA RACE",
		"Read at 0x00c0000e4030 by goroutine 7:",
		"  main.panicRace.func1()",
		"      /go/src/github.com/maruel/panicparse/cmd/panic/main_race.go:37 +0x38",
		"",
		"Previous write at 0x00c0000e4030 by goroutine 6:",
		"  main.panicRace.func1()",
		"      /go/src/github.com/maruel/panicparse/cmd/panic/main_race.go:37 +0x4e",
		"",
		"Goroutine 7 (running) created at:",
		"  main.panicRace()",
		"      /go/src/github.com/maruel/panicparse/cmd/panic/main_race.go:35 +0x88",
		"  main.main()",
		"      /go/src/github.com/maruel/panicparse/cmd/panic/main.go:252 +0x2d9",
		"",
		"Goroutine 6 (running) created at:",
		"  main.panicRace()",
		"      /go/src/github.com/maruel/panicparse/cmd/panic/main_race.go:35 +0x88",
		"  main.main()",
		"      /go/src/github.com/maruel/panicparse/cmd/panic/main.go:252 +0x2d9",
		"==================",
		"",
	}
	extra := &bytes.Buffer{}
	expected := []*Goroutine{
		{
			Signature: Signature{
				State: "running",
				Stack: Stack{
					Calls: []Call{
						{
							SrcPath: "/go/src/github.com/maruel/panicparse/cmd/panic/main.go",
							Line:    252,
							Func:    Func{Raw: "main.panicRace"},
						},
					},
				},
			},
			ID:    7,
			First: true,
		},
		{
			Signature: Signature{
				State: "running",
				Stack: Stack{
					Calls: []Call{
						{
							SrcPath: "/go/src/github.com/maruel/panicparse/cmd/panic/main.go",
							Line:    252,
							Func:    Func{Raw: "main.panicRace"},
						},
					},
				},
			},
			ID: 6,
		},
	}
	scanner := bufio.NewScanner(bytes.NewBufferString(strings.Join(data, "\n")))
	scanner.Split(scanLines)
	s := scanningState{raceDetectionEnabled: true}
	for scanner.Scan() {
		line, err := s.scan(scanner.Text())
		if line != "" {
			_, _ = io.WriteString(extra, line)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	compareGoroutines(t, expected, s.goroutines)
	expectedOps := []raceOp{{false, 0xc0000e4030, 7}, {true, 0xc0000e4030, 6}}
	if !reflect.DeepEqual(expectedOps, s.races) {
		t.Fatalf("%v", s.races)
	}
}

func TestSplitPath(t *testing.T) {
	if p := splitPath(""); p != nil {
		t.Fatalf("expected nil, got: %v", p)
	}
}

func TestGetGOPATHS(t *testing.T) {
	old := os.Getenv("GOPATH")
	defer func() {
		os.Setenv("GOPATH", old)
	}()
	os.Setenv("GOPATH", "")
	if p := getGOPATHs(); len(p) != 1 {
		t.Fatalf("expected only one path: %v", p)
	}
}

// Test runtime code. For now just assert that they succeed (beside race).
// Later they'll be used for the actual expectations instead of the hardcoded
// ones above.
func TestPanic(t *testing.T) {
	cmds := internaltest.PanicOutputs()
	expected := map[string]int{
		"chan_receive":              2,
		"chan_send":                 2,
		"goroutine_1":               2,
		"goroutine_dedupe_pointers": 101,
		"goroutine_100":             101,
	}

	panicParseDir := getPanicParseDir(t)
	ppDir := pathJoin(panicParseDir, "cmd", "panic")

	custom := map[string]func(*testing.T, *Context, *bytes.Buffer, string){
		"args_elided": testPanicArgsElided,
		"mismatched":  testPanicMismatched,
		"str":         testPanicStr,
		"utf8":        testPanicUTF8,
	}
	// Make sure all custom handlers are showing up in cmds.
	for n := range custom {
		if _, ok := cmds[n]; !ok {
			t.Fatalf("untested mode: %q in:\n%v", n, cmds)
		}
	}

	for cmd, data := range cmds {
		t.Run(cmd, func(t *testing.T) {
			b := bytes.Buffer{}
			c, err := ParseDump(bytes.NewReader(data), &b, true)
			if err != nil {
				t.Fatal(err)
			}
			if cmd == "race" {
				// TODO(maruel): Fix this.
				if c != nil {
					t.Fatal("unexpected context")
				}
				return
			}

			if c == nil {
				t.Fatal("context is nil")
			}
			if f := custom[cmd]; f != nil {
				f(t, c, &b, ppDir)
				return
			}
			if c.GOROOT != runtime.GOROOT() {
				//t.Logf("GOROOT is %q", c.GOROOT)
			}
			e := expected[cmd]
			if e == 0 {
				e = 1
			}
			if actual := len(c.Goroutines); actual != e {
				t.Fatalf("unexpected Goroutines; expected %d, got %d", e, actual)
			}
		})
	}
}

func testPanicArgsElided(t *testing.T, c *Context, b *bytes.Buffer, ppDir string) {
	if c.GOROOT != "" {
		t.Fatalf("GOROOT is %q", c.GOROOT)
	}
	if b.String() != "GOTRACEBACK=all\npanic: 1\n\n" {
		t.Fatalf("output: %q", b.String())
	}
	expected := []*Goroutine{
		{
			Signature: Signature{
				State: "running",
				Stack: Stack{
					Calls: []Call{
						{
							SrcPath:      pathJoin(ppDir, "main.go"),
							LocalSrcPath: pathJoin(ppDir, "main.go"),
							RelSrcPath:   "github.com/maruel/panicparse/cmd/panic/main.go",
							Line:         58,
							Func:         Func{Raw: "main.panicArgsElided"},
							Args: Args{
								Values: []Arg{{Value: 1}, {Value: 2}, {Value: 3}, {Value: 4}, {Value: 5}, {Value: 6}, {Value: 7}, {Value: 8}, {Value: 9}, {Value: 10}},
								Elided: true,
							},
						},
						{
							SrcPath:      pathJoin(ppDir, "main.go"),
							LocalSrcPath: pathJoin(ppDir, "main.go"),
							RelSrcPath:   "github.com/maruel/panicparse/cmd/panic/main.go",
							Line:         134,
							Func:         Func{Raw: "main.glob..func1"},
						},
						{
							SrcPath:      pathJoin(ppDir, "main.go"),
							LocalSrcPath: pathJoin(ppDir, "main.go"),
							RelSrcPath:   "github.com/maruel/panicparse/cmd/panic/main.go",
							Line:         340,
							Func:         Func{Raw: "main.main"},
						},
					},
				},
			},
			ID:    1,
			First: true,
		},
	}
	similarGoroutines(t, expected, c.Goroutines)
}

func testPanicMismatched(t *testing.T, c *Context, b *bytes.Buffer, ppDir string) {
	if c.GOROOT != "" {
		t.Fatalf("GOROOT is %q", c.GOROOT)
	}
	if b.String() != "GOTRACEBACK=all\npanic: 42\n\n" {
		t.Fatalf("output: %q", b.String())
	}
	expected := []*Goroutine{
		{
			Signature: Signature{
				State: "running",
				Stack: Stack{
					Calls: []Call{
						{
							SrcPath:      pathJoin(ppDir, "internal", "incorrect", "correct.go"),
							LocalSrcPath: pathJoin(ppDir, "internal", "incorrect", "correct.go"),
							RelSrcPath:   "github.com/maruel/panicparse/cmd/panic/internal/incorrect/correct.go",
							Line:         7,
							// This is important to note here that the Go runtime prints out
							// the package path, and not the package name.
							//
							// Here the package name is "correct". There is no way to deduce
							// this from the stack trace.
							Func: Func{Raw: "github.com/maruel/panicparse/cmd/panic/internal/incorrect.Panic"},
						},
						{
							SrcPath:      pathJoin(ppDir, "main.go"),
							LocalSrcPath: pathJoin(ppDir, "main.go"),
							RelSrcPath:   "github.com/maruel/panicparse/cmd/panic/main.go",
							Line:         314,
							Func:         Func{Raw: "main.glob..func18"},
						},
						{
							SrcPath:      pathJoin(ppDir, "main.go"),
							LocalSrcPath: pathJoin(ppDir, "main.go"),
							RelSrcPath:   "github.com/maruel/panicparse/cmd/panic/main.go",
							Line:         340,
							Func:         Func{Raw: "main.main"},
						},
					},
				},
			},
			ID:    1,
			First: true,
		},
	}
	similarGoroutines(t, expected, c.Goroutines)
}

func testPanicStr(t *testing.T, c *Context, b *bytes.Buffer, ppDir string) {
	if c.GOROOT != "" {
		t.Fatalf("GOROOT is %q", c.GOROOT)
	}
	if b.String() != "GOTRACEBACK=all\npanic: allo\n\n" {
		t.Fatalf("output: %q", b.String())
	}
	expected := []*Goroutine{
		{
			Signature: Signature{
				State: "running",
				Stack: Stack{
					Calls: []Call{
						{
							SrcPath:      pathJoin(ppDir, "main.go"),
							LocalSrcPath: pathJoin(ppDir, "main.go"),
							RelSrcPath:   "github.com/maruel/panicparse/cmd/panic/main.go",
							Line:         50,
							Func:         Func{Raw: "main.panicstr"},
							Args: Args{
								Values: []Arg{{Value: 0x123456}, {Value: 4}},
							},
						},
						{
							SrcPath:      pathJoin(ppDir, "main.go"),
							LocalSrcPath: pathJoin(ppDir, "main.go"),
							RelSrcPath:   "github.com/maruel/panicparse/cmd/panic/main.go",
							Line:         307,
							Func:         Func{Raw: "main.glob..func17"},
						},
						{
							SrcPath:      pathJoin(ppDir, "main.go"),
							LocalSrcPath: pathJoin(ppDir, "main.go"),
							RelSrcPath:   "github.com/maruel/panicparse/cmd/panic/main.go",
							Line:         340,
							Func:         Func{Raw: "main.main"},
						},
					},
				},
			},
			ID:    1,
			First: true,
		},
	}
	similarGoroutines(t, expected, c.Goroutines)
}

func testPanicUTF8(t *testing.T, c *Context, b *bytes.Buffer, ppDir string) {
	if c.GOROOT != "" {
		t.Fatalf("GOROOT is %q", c.GOROOT)
	}
	if b.String() != "GOTRACEBACK=all\npanic: 42\n\n" {
		t.Fatalf("output: %q", b.String())
	}
	expected := []*Goroutine{
		{
			Signature: Signature{
				State: "running",
				Stack: Stack{
					Calls: []Call{
						{
							// See TestCallUTF8 in stack_test.go for exercising the methods on
							// Call in this situation.
							SrcPath:      pathJoin(ppDir, "internal", "ùtf8", "ùtf8.go"),
							LocalSrcPath: pathJoin(ppDir, "internal", "ùtf8", "ùtf8.go"),
							RelSrcPath:   "github.com/maruel/panicparse/cmd/panic/internal/ùtf8/ùtf8.go",
							Line:         10,
							// This is important to note here the inconsistency in the Go
							// runtime stack generator. The path is escaped, but symbols are
							// not.
							Func: Func{Raw: "github.com/maruel/panicparse/cmd/panic/internal/%c3%b9tf8.(*Strùct).Pànic"},
							Args: Args{Values: []Arg{{Value: 0xc0000b2e48}}},
						},
						{
							SrcPath:      pathJoin(ppDir, "main.go"),
							LocalSrcPath: pathJoin(ppDir, "main.go"),
							RelSrcPath:   "github.com/maruel/panicparse/cmd/panic/main.go",
							Line:         322,
							Func:         Func{Raw: "main.glob..func19"},
						},
						{
							SrcPath:      pathJoin(ppDir, "main.go"),
							LocalSrcPath: pathJoin(ppDir, "main.go"),
							RelSrcPath:   "github.com/maruel/panicparse/cmd/panic/main.go",
							Line:         340,
							Func:         Func{Raw: "main.main"},
						},
					},
				},
			},
			ID:    1,
			First: true,
		},
	}
	similarGoroutines(t, expected, c.Goroutines)
}

// TestPanicweb implements the parsing of panicweb output.
//
// panicweb is a separate binary from the rest of panic because importing the
// "net" package causes a background thread to be started, which breaks "panic
// asleep".
func TestPanicweb(t *testing.T) {
	t.Parallel()
	b := bytes.Buffer{}
	c, err := ParseDump(bytes.NewReader(internaltest.PanicwebOutput()), &b, true)
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("context is nil")
	}
	if b.String() != "panic: Here's a snapshot of a normal web server.\n\n" {
		t.Fatalf("output: %q", b.String())
	}
	if c.GOROOT != strings.Replace(runtime.GOROOT(), "\\", "/", -1) {
		t.Fatalf("GOROOT mismatch; expected:%q detected:%q", runtime.GOROOT(), c.GOROOT)
	}
	if actual := len(c.Goroutines); actual < 30 {
		t.Fatalf("unexpected Goroutines; expected at least 30, got %d", actual)
	}
	// Reduce the goroutines.
	actual := Aggregate(c.Goroutines, AnyPointer)
	// The goal here is not to find the exact match since it'll change across
	// OSes and Go versions, but to find some of the expected signatures.
	pwebDir := pathJoin(getPanicParseDir(t), "cmd", "panicweb")
	// Categorize the signatures.
	var types []panicwebSignatureType
	for _, b := range actual {
		types = append(types, identifyPanicwebSignature(t, b, pwebDir))
	}
	// Count the expected types.
	if v := pstCount(types, pstUnknown); v != 0 {
		t.Fatalf("found %d unknown signatures", v)
	}
	if v := pstCount(types, pstMain); v != 1 {
		t.Fatalf("found %d pstMain signatures", v)
	}
	if v := pstCount(types, pstURL1handler); v != 1 {
		t.Fatalf("found %d URL1Handler signatures", v)
	}
	if v := pstCount(types, pstURL2handler); v != 1 {
		t.Fatalf("found %d URL2Handler signatures", v)
	}
	if v := pstCount(types, pstClient); v < 2 {
		t.Fatalf("found %d client signatures", v)
	}
	if v := pstCount(types, pstServe); v != 1 {
		t.Fatalf("found %d serve signatures", v)
	}
	if v := pstCount(types, pstColorable); v != 1 {
		t.Fatalf("found %d colorable signatures", v)
	}
	if v := pstCount(types, pstStdlib); v < 3 {
		t.Fatalf("found %d stdlib signatures", v)
	}
}

func BenchmarkParseDump_Guess(b *testing.B) {
	b.ReportAllocs()
	data := internaltest.PanicwebOutput()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c, err := ParseDump(bytes.NewReader(data), ioutil.Discard, true)
		if err != nil {
			b.Fatal(err)
		}
		if c == nil {
			b.Fatal("missing context")
		}
	}
}

func BenchmarkParseDump_NoGuess(b *testing.B) {
	b.ReportAllocs()
	data := internaltest.PanicwebOutput()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c, err := ParseDump(bytes.NewReader(data), ioutil.Discard, false)
		if err != nil {
			b.Fatal(err)
		}
		if c == nil {
			b.Fatal("missing context")
		}
	}
}

//

type panicwebSignatureType int

const (
	pstUnknown panicwebSignatureType = iota
	pstMain
	pstURL1handler
	pstURL2handler
	pstClient
	pstServe
	pstColorable
	pstStdlib
)

func pstCount(s []panicwebSignatureType, t panicwebSignatureType) int {
	i := 0
	for _, v := range s {
		if v == t {
			i++
		}
	}
	return i
}

func identifyPanicwebSignature(t *testing.T, b *Bucket, pwebDir string) panicwebSignatureType {
	// The first bucket (the one calling panic()) is deterministic.
	if b.First {
		if len(b.IDs) != 1 {
			t.Fatal("first bucket is not correct")
			return pstUnknown
		}
		crash := Signature{
			State: "running",
			Stack: Stack{
				Calls: []Call{
					{
						SrcPath:      pathJoin(pwebDir, "main.go"),
						LocalSrcPath: pathJoin(pwebDir, "main.go"),
						Line:         68,
						Func:         Func{Raw: "main.main"},
						RelSrcPath:   "github.com/maruel/panicparse/cmd/panicweb/main.go",
					},
				},
			},
		}
		compareSignatures(t, &crash, &b.Signature)
		return pstMain
	}

	// We should find exactly 10 sleeping routines in the URL1Handler handler
	// signature and 3 in URL2Handler.
	if s := b.Stack.Calls[0].Func.Name(); s == "URL1Handler" || s == "URL2Handler" {
		if b.State != "chan receive" {
			t.Fatalf("suspicious: %#v", b)
			return pstUnknown
		}
		if b.Stack.Calls[0].ImportPath() != "github.com/maruel/panicparse/cmd/panicweb/internal" {
			t.Fatalf("suspicious: %#v", b)
			return pstUnknown
		}
		if b.Stack.Calls[0].SrcName() != "internal.go" {
			t.Fatalf("suspicious: %#v", b)
			return pstUnknown
		}
		if b.CreatedBy.SrcName() != "server.go" {
			t.Fatalf("suspicious: %#v", b)
			return pstUnknown
		}
		if b.CreatedBy.Func.PkgDotName() != "http.(*Server).Serve" {
			t.Fatalf("suspicious: %#v", b)
			return pstUnknown
		}
		if s == "URL1Handler" {
			return pstURL1handler
		}
		return pstURL2handler
	}

	// Find the client goroutine signatures. For the client, it is likely that
	// they haven't all bucketed perfectly.
	if b.CreatedBy.Func.PkgDotName() == "internal.GetAsync" {
		// TODO(maruel): More checks.
		return pstClient
	}

	// Now find the two goroutine started by main.
	if b.CreatedBy.Func.PkgDotName() == "main.main" {
		if b.State == "IO wait" {
			return pstServe
		}
		if b.State == "chan receive" {
			localgopath := getGOPATHs()[0]
			// If not using Go modules, the path is different as the vendored version
			// is used instead.
			pColorable := "pkg/mod/github.com/mattn/go-colorable@v0.1.6/noncolorable.go"
			pkgPrefix := ""
			pRelColorable := "github.com/mattn/go-colorable@v0.1.6/noncolorable.go"
			if !isUsingModules(t) {
				t.Logf("Using vendored")
				pRelColorable = "github.com/maruel/panicparse/vendor/github.com/mattn/go-colorable/noncolorable.go"
				pColorable = "src/" + pRelColorable
				pkgPrefix = "github.com/maruel/panicparse/vendor/"
			} else {
				t.Logf("Using go module")
			}
			expected := Signature{
				State: "chan receive",
				Stack: Stack{
					Calls: []Call{
						{
							SrcPath:      pathJoin(pwebDir, "main.go"),
							LocalSrcPath: pathJoin(pwebDir, "main.go"),
							RelSrcPath:   "github.com/maruel/panicparse/cmd/panicweb/main.go",
							Line:         79,
							Func:         Func{Raw: "main.(*writeHang).Write"},
							Args:         Args{Values: []Arg{{}, {}, {}, {}, {}, {}, {}}},
						},
						{
							SrcPath:      pathJoin(localgopath, pColorable),
							LocalSrcPath: pathJoin(localgopath, pColorable),
							RelSrcPath:   pRelColorable,
							Line:         30,
							Func:         Func{Raw: pkgPrefix + "github.com/mattn/go-colorable.(*NonColorable).Write"},
							Args:         Args{Values: []Arg{{}, {}, {}, {}, {}, {}, {}}},
						},
					},
				},
				CreatedBy: Call{
					SrcPath:      pathJoin(pwebDir, "main.go"),
					LocalSrcPath: pathJoin(pwebDir, "main.go"),
					RelSrcPath:   "github.com/maruel/panicparse/cmd/panicweb/main.go",
					Line:         61,
					Func:         Func{Raw: "main.main"},
				},
			}
			// The arguments content is variable, so just count the number of
			// arguments and give up on the rest.
			for i := range b.Signature.Stack.Calls {
				for j := range b.Signature.Stack.Calls[i].Args.Values {
					b.Signature.Stack.Calls[i].Args.Values[j].Value = 0
					b.Signature.Stack.Calls[i].Args.Values[j].Name = ""
				}
			}
			compareSignatures(t, &expected, &b.Signature)
			return pstColorable
		}
		t.Fatalf("suspicious: %#v", b)
		return pstUnknown
	}

	// The rest should all be created with internal threads.
	if b.CreatedBy.IsStdlib {
		return pstStdlib
	}

	// On older Go version, there's often an assembly stack in asm_amd64.s.
	if b.CreatedBy.Func.Raw == "" {
		if len(b.Stack.Calls) == 1 && b.Stack.Calls[0].Func.Raw == "runtime.goexit" {
			return pstStdlib
		}
	}
	t.Fatalf("unexpected thread started by non-stdlib: %# v", b)
	return pstUnknown
}

//

// isUsingModules is best guess to know if go module are enabled.
func isUsingModules(t *testing.T) bool {
	def := false
	if ver := runtime.Version(); strings.HasPrefix(ver, "devel ") {
		t.Logf("build %q; assuming a recent version", ver)
		def = true
	} else if strings.HasPrefix(ver, "go1.") {
		v := ver[4:]
		// Only keep the major version. In practice for Go 1.9 and 1.10 we should
		// check the minor version too. Please submit a PR if you need to handle
		// this case.
		if i := strings.IndexByte(v, '.'); i != -1 {
			v = v[:i]
		}
		if m, err := strconv.Atoi(v); m >= 14 {
			def = true
		} else if err != nil {
			t.Errorf("failed to parse %q: %v", ver, err)
		}
	} else {
		// This will break on go2. Please submit a PR to fix this once Go2 is
		// released.
		t.Fatalf("unexpected go version %q", ver)
	}
	s := os.Getenv("GO111MODULE")
	return (def && (s == "auto" || s == "")) || s == "on"
}

// getPanicParseDir returns the path to the root directory of panicparse
// package, using "/" as path separator.
func getPanicParseDir(t *testing.T) string {
	// We assume that the working directory is the directory containing this
	// source. In Go test framework, this normally holds true. If this ever
	// becomes false, let's fix this.
	thisDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// "/" is used even on Windows in the stack trace, return in this format to
	// simply our life.
	return strings.Replace(filepath.Dir(thisDir), "\\", "/", -1)
}
