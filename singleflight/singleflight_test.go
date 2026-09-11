// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package singleflight

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

type errValue struct{}

func (err *errValue) Error() string {
	return "error value"
}

func TestPanicErrorUnwrap(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		panicValue       any
		wrappedErrorType bool
	}{
		{
			name:             "panicError wraps non-error type",
			panicValue:       &panicError{value: "string value"},
			wrappedErrorType: false,
		},
		{
			name:             "panicError wraps error type",
			panicValue:       &panicError{value: new(errValue)},
			wrappedErrorType: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var recovered any

			group := &Group[string, any]{}

			func() {
				defer func() {
					recovered = recover()
					t.Logf("after panic(%#v) in group.Do, recovered %#v", tc.panicValue, recovered)
				}()

				_, _, _ = group.Do(tc.name, func() (any, error) {
					panic(tc.panicValue)
				})
			}()

			if recovered == nil {
				t.Fatal("expected a non-nil panic value")
			}

			err, ok := recovered.(error)
			if !ok {
				t.Fatalf("recovered non-error type: %T", recovered)
			}

			if !errors.Is(err, new(errValue)) && tc.wrappedErrorType {
				t.Errorf("unexpected wrapped error type %T; want %T", err, new(errValue))
			}
		})
	}
}

func TestDo(t *testing.T) {
	var g Group[string, string]
	v, err, _ := g.Do("key", func() (string, error) {
		return "bar", nil
	})
	if got, want := fmt.Sprintf("%v (%T)", v, v), "bar (string)"; got != want {
		t.Errorf("Do = %v; want %v", got, want)
	}
	if err != nil {
		t.Errorf("Do error = %v", err)
	}
}

func TestDoErr(t *testing.T) {
	var g Group[string, string]
	someErr := errors.New("Some error")
	v, err, _ := g.Do("key", func() (string, error) {
		return "", someErr
	})
	if !errors.Is(err, someErr) {
		t.Errorf("Do error = %v; want someErr %v", err, someErr)
	}
	if v != "" {
		t.Errorf("unexpected non-zero value %#v", v)
	}
}

func TestDoDupSuppress(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var g Group[string, string]
		var calls atomic.Int32
		block := make(chan struct{})
		fn := func() (string, error) {
			calls.Add(1)
			<-block
			return "bar", nil
		}

		const n = 10
		var wg sync.WaitGroup
		for range n {
			wg.Go(func() {
				v, err, shared := g.Do("key", fn)
				if err != nil {
					t.Errorf("Do error: %v", err)
					return
				}
				if v != "bar" {
					t.Errorf("Do = %q; want %q", v, "bar")
				}
				if !shared {
					t.Error("Do shared = false; want true")
				}
			})
		}

		// Returns once every caller is inside Do, one running fn and the rest waiting on it.
		synctest.Wait()
		close(block)
		wg.Wait()

		if got := calls.Load(); got != 1 {
			t.Errorf("number of calls = %d; want 1", got)
		}
	})
}

// Test that singleflight behaves correctly after Forget called.
// See https://github.com/golang/go/issues/31420
func TestForget(t *testing.T) {
	var g Group[string, int]

	var (
		firstStarted  = make(chan struct{})
		unblockFirst  = make(chan struct{})
		firstFinished = make(chan struct{})
	)

	go func() {
		g.Do("key", func() (int, error) {
			close(firstStarted)
			<-unblockFirst
			close(firstFinished)
			return 0, nil
		})
	}()
	<-firstStarted
	g.Forget("key")

	unblockSecond := make(chan struct{})
	secondResult := g.DoChan("key", func() (int, error) {
		<-unblockSecond
		return 2, nil
	})

	close(unblockFirst)
	<-firstFinished

	thirdResult := g.DoChan("key", func() (int, error) {
		return 3, nil
	})

	close(unblockSecond)
	<-secondResult
	r := <-thirdResult
	if r.Val != 2 {
		t.Errorf("We should receive result produced by second call, expected: 2, got %d", r.Val)
	}
}

func TestDoChan(t *testing.T) {
	var g Group[string, string]
	ch := g.DoChan("key", func() (string, error) {
		return "bar", nil
	})

	res := <-ch
	v := res.Val
	err := res.Err
	if got, want := fmt.Sprintf("%v (%T)", v, v), "bar (string)"; got != want {
		t.Errorf("Do = %v; want %v", got, want)
	}
	if err != nil {
		t.Errorf("Do error = %v", err)
	}
}

// Test singleflight behaves correctly after Do panic.
// See https://github.com/golang/go/issues/41133
func TestPanicDo(t *testing.T) {
	var g Group[string, string]
	fn := func() (string, error) {
		panic("invalid memory address or nil pointer dereference")
	}

	const n = 5
	var waited, panicCount atomic.Int32
	waited.Store(n)
	done := make(chan struct{})
	for range n {
		go func() {
			defer func() {
				if err := recover(); err != nil {
					t.Logf("Got panic: %v\n%s", err, debug.Stack())
					panicCount.Add(1)
				}

				if waited.Add(-1) == 0 {
					close(done)
				}
			}()

			g.Do("key", fn)
		}()
	}

	select {
	case <-done:
		if got := panicCount.Load(); got != n {
			t.Errorf("Expect %d panic, but got %d", n, got)
		}
	case <-time.After(time.Second):
		t.Fatalf("Do hangs")
	}
}

func TestGoexitDo(t *testing.T) {
	var g Group[string, string]
	fn := func() (string, error) {
		runtime.Goexit()
		return "", nil
	}

	const n = 5
	var waited atomic.Int32
	waited.Store(n)
	done := make(chan struct{})
	for range n {
		go func() {
			var err error
			defer func() {
				if err != nil {
					t.Errorf("Error should be nil, but got: %v", err)
				}
				if waited.Add(-1) == 0 {
					close(done)
				}
			}()
			_, err, _ = g.Do("key", fn)
		}()
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("Do hangs")
	}
}

func executable(t testing.TB) string {
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("skipping: test executable not found")
	}

	// Control case: check whether exec.Command works at all.
	// (For example, it might fail with a permission error on iOS.)
	cmd := exec.Command(exe, "-test.list=^$")
	cmd.Env = []string{}
	if err := cmd.Run(); err != nil {
		t.Skipf("skipping: exec appears not to work on %s: %v", runtime.GOOS, err)
	}

	return exe
}

func TestPanicDoChan(t *testing.T) {
	if os.Getenv("TEST_PANIC_DOCHAN") != "" {
		defer func() {
			recover()
		}()

		g := new(Group[string, string])
		ch := g.DoChan("", func() (string, error) {
			panic("Panicking in DoChan")
		})
		<-ch
		t.Fatalf("DoChan unexpectedly returned")
	}

	t.Parallel()

	cmd := exec.Command(executable(t), "-test.run="+t.Name(), "-test.v")
	cmd.Env = append(os.Environ(), "TEST_PANIC_DOCHAN=1")
	out := new(bytes.Buffer)
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	err := cmd.Wait()
	t.Logf("%s:\n%s", strings.Join(cmd.Args, " "), out)
	if err == nil {
		t.Errorf("Test subprocess passed; want a crash due to panic in DoChan")
	}
	if bytes.Contains(out.Bytes(), []byte("DoChan unexpectedly")) {
		t.Errorf("Test subprocess failed with an unexpected failure mode.")
	}
	if !bytes.Contains(out.Bytes(), []byte("Panicking in DoChan")) {
		t.Errorf("Test subprocess failed, but the crash isn't caused by panicking in DoChan")
	}
}

func TestPanicDoSharedByDoChan(t *testing.T) {
	if os.Getenv("TEST_PANIC_DOCHAN") != "" {
		blocked := make(chan struct{})
		unblock := make(chan struct{})

		g := new(Group[string, string])
		go func() {
			defer func() {
				recover()
			}()
			g.Do("", func() (string, error) {
				close(blocked)
				<-unblock
				panic("Panicking in Do")
			})
		}()

		<-blocked
		ch := g.DoChan("", func() (string, error) {
			panic("DoChan unexpectedly executed callback")
		})
		close(unblock)
		<-ch
		t.Fatalf("DoChan unexpectedly returned")
	}

	t.Parallel()

	cmd := exec.Command(executable(t), "-test.run="+t.Name(), "-test.v")
	cmd.Env = append(os.Environ(), "TEST_PANIC_DOCHAN=1")
	out := new(bytes.Buffer)
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	err := cmd.Wait()
	t.Logf("%s:\n%s", strings.Join(cmd.Args, " "), out)
	if err == nil {
		t.Errorf("Test subprocess passed; want a crash due to panic in Do shared by DoChan")
	}
	if bytes.Contains(out.Bytes(), []byte("DoChan unexpectedly")) {
		t.Errorf("Test subprocess failed with an unexpected failure mode.")
	}
	if !bytes.Contains(out.Bytes(), []byte("Panicking in Do")) {
		t.Errorf("Test subprocess failed, but the crash isn't caused by panicking in Do")
	}
}
