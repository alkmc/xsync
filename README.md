# xsync

Generic variants of the concurrency primitives from
[golang.org/x/sync](https://pkg.go.dev/golang.org/x/sync).

```sh
go get github.com/alkmc/xsync
```

## singleflight

Duplicate function call suppression: concurrent callers asking for the same key
share a single execution of the given function.

Upstream returns `any`, so every caller has to assert the result type. Here the
value is a type parameter and keys stay `string`:

```go
var g singleflight.Group[*User]

user, err, shared := g.Do(userID, func() (*User, error) {
    return fetchUser(ctx, userID)
})
```

`Do`, `DoChan` and `Forget` behave exactly as in
`golang.org/x/sync/singleflight`, including the handling of panics and
`runtime.Goexit` inside the given function.

## Provenance

The `singleflight` package is derived from
[golang.org/x/sync/singleflight](https://github.com/golang/sync) v0.23.0,
revision `f75267d8412fc1dfd12b343644a7ea46e4d9c85d`, released 2026-08-31.
Copyright The Go Authors, licensed under the BSD 3-Clause License. See
`LICENSE` and `PATENTS`.

It was parameterized over the value type in place of `any` values, and its
error handling was updated for Go 1.27. This repository is not affiliated with
or endorsed by Google or the Go project.
