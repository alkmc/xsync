// Copyright 2026 Aleksander Kmiecik. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package singleflight

import (
	"errors"
	"sync/atomic"
	"testing"
)

type cacheKey struct {
	tenant string
	id     int
}

type user struct {
	ID   int
	Name string
}

// Equal struct keys share one in-flight call even though they are distinct values.
func TestDoStructKeySuppress(t *testing.T) {
	var g Group[cacheKey, string]

	started := make(chan struct{})
	unblock := make(chan struct{})
	var calls atomic.Int32

	first := g.DoChan(cacheKey{tenant: "acme", id: 7}, func() (string, error) {
		calls.Add(1)
		close(started)
		<-unblock
		return "value", nil
	})
	<-started

	second := g.DoChan(cacheKey{tenant: "acme", id: 7}, func() (string, error) {
		calls.Add(1)
		return "unexpected", nil
	})
	close(unblock)

	for _, ch := range []<-chan Result[string]{first, second} {
		r := <-ch
		if r.Err != nil {
			t.Fatalf("Err = %v; want nil", r.Err)
		}
		if r.Val != "value" {
			t.Errorf("Val = %q; want %q", r.Val, "value")
		}
		if !r.Shared {
			t.Error("Shared = false; want true")
		}
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("number of calls = %d; want 1", got)
	}
}

// Duplicate callers receive the error and the zero value of a pointer type.
func TestDoChanErrShared(t *testing.T) {
	var g Group[string, *user]
	wantErr := errors.New("boom")

	started := make(chan struct{})
	unblock := make(chan struct{})

	first := g.DoChan("key", func() (*user, error) {
		close(started)
		<-unblock
		return nil, wantErr
	})
	<-started

	second := g.DoChan("key", func() (*user, error) {
		return &user{ID: 1, Name: "unexpected"}, nil
	})
	close(unblock)

	for _, ch := range []<-chan Result[*user]{first, second} {
		r := <-ch
		if !errors.Is(r.Err, wantErr) {
			t.Errorf("Err = %v; want %v", r.Err, wantErr)
		}
		if r.Val != nil {
			t.Errorf("Val = %v; want nil", r.Val)
		}
	}
}

// Completed calls must not be retained by the group.
func TestCallCleanup(t *testing.T) {
	var g Group[int, string]

	if _, err, _ := g.Do(1, func() (string, error) { return "a", nil }); err != nil {
		t.Fatalf("Do error = %v", err)
	}
	if r := <-g.DoChan(2, func() (string, error) { return "b", nil }); r.Err != nil {
		t.Fatalf("DoChan error = %v", r.Err)
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.m) != 0 {
		t.Errorf("in-flight calls = %d; want 0", len(g.m))
	}
}
