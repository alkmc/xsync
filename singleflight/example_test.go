// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package singleflight

import "fmt"

func ExampleGroup() {
	g := new(Group[string, string])

	block := make(chan struct{})
	res1c := g.DoChan("key", func() (string, error) {
		<-block
		return "func 1", nil
	})
	res2c := g.DoChan("key", func() (string, error) {
		<-block
		return "func 2", nil
	})
	close(block)

	res1 := <-res1c
	res2 := <-res2c

	// Results are shared by functions executed with duplicate keys.
	fmt.Println("Shared:", res2.Shared)
	// Only the first function is executed: it is registered and started with "key",
	// and doesn't complete before the second function is registered with a duplicate key.
	fmt.Println("Equal results:", res1.Val == res2.Val)
	fmt.Println("Result:", res1.Val)

	// Output:
	// Shared: true
	// Equal results: true
	// Result: func 1
}
