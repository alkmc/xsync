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
[golang.org/x/sync/singleflight](https://github.com/golang/sync) v0.24.0,
revision `36f2d70ecde9857dc4e363242a1369c7645d6770`, released 2026-09-23.
Copyright The Go Authors, licensed under the BSD 3-Clause License. See
`LICENSE` and `PATENTS`.

It was parameterized over the value type in place of `any` values, and its
error handling was updated for Go 1.27. This repository is not affiliated with
or endorsed by Google or the Go project.
