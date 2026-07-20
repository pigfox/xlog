# xlog

Thread-safe JSON logger with a short global API.

## Install

```bash
go get github.com/pigfox/xlog@v1.0.0
```

## Initialize

Call `Init()` once at process startup. If you never do, the first log call
lazily installs a logger writing to stdout.

```go
package main

import (
	"errors"
	"os"

	"github.com/pigfox/xlog"
)

func main() {
	xlog.Init(os.Stdout)
	xlog.SetLevel(xlog.LevelInfo)

	xlog.Info("server starting")
	xlog.Infof("listening on port %d", 8080)
	xlog.Error2("db", errors.New("connect failed"))

	log := xlog.With("request_id", "abc123", "route", "/v1/users")
	log.Warn("slow response")
}
```

## API

| Function | Level | Notes |
| --- | --- | --- |
| `Init(w io.Writer)` | — | nil → stdout |
| `SetLevel(l Level)` | — | safe to call concurrently with logging |
| `Info(msg string)` | INFO | |
| `Infof(format string, a ...any)` | INFO | |
| `Warn(msg string)` | WARN | |
| `Warn2(prefix string, err error)` | WARN | `prefix + ": " + err`; no-op if err is nil |
| `Error(err error)` | ERROR | no-op if err is nil |
| `Error2(prefix string, err error)` | ERROR | `prefix + ": " + err`; no-op if err is nil |
| `Errorf(format string, a ...any)` | ERROR | |
| `With(kv ...any) *Logger` | — | child logger carrying extra key/value pairs |

`Level` aliases `slog.Level`; the constants are `LevelDebug`, `LevelInfo`,
`LevelWarn`, `LevelError`.

`*Logger` has the same methods — `Info`, `Infof`, `Warn`, `Warn2`, `Error`,
`Error2`, `Errorf` — plus `With` for further chaining.

## Output format

Keys are emitted in this fixed order:

```json
{"caller":"pkg.file.go line 123","level":"ERROR","msg":"something failed","ts":"2026-03-03T10:50:25.389260945-08:00"}
```

`With` attributes are appended after the four envelope keys, in the order they
were added:

```json
{"caller":"api.handler.go line 42","level":"WARN","msg":"slow response","ts":"2026-03-03T10:50:25.389260945-08:00","request_id":"abc123","route":"/v1/users"}
```

The envelope always wins. A `With` key that collides with `caller`, `level`,
`msg`, or `ts` is renamed with an `attr_` prefix, so a line never carries
duplicate JSON keys and the envelope can't be spoofed:

```go
xlog.With("level", "fake").Info("real")
// {"caller":"...","level":"INFO","msg":"real","ts":"...","attr_level":"fake"}
```

Repeated `With` keys are emitted repeatedly, matching `log/slog` — xlog does
not deduplicate them. Only the four envelope keys are protected:

```go
xlog.With("a", 1).With("a", 2).Info("x")
// {"caller":"...","level":"INFO","msg":"x","ts":"...","a":1,"a":2}
```

An odd number of arguments to `With` follows `log/slog`: the orphan is emitted
under the `!BADKEY` marker rather than panicking or dropping the line.

```go
xlog.With("orphan").Info("x")
// {"caller":"...","level":"INFO","msg":"x","ts":"...","!BADKEY":"orphan"}
```

Caller behavior:

- When called from `main`, `caller` points at your `main.go`.
- When called from another package, `caller` points at the calling package's file/line.
- `With(...)` loggers resolve the same call site as the package-level functions.

## Migrating from v0

v1 output is **byte-compatible with v0** for `Init`, `Info`, `Error`, and
`Error2` — same keys, same order, same prefix joining, and the same silent
no-op when the error is nil. Those four signatures are unchanged, so existing
call sites compile and log identically.

New functions extend the same line format: `Warn`/`Warn2` reuse the existing
`level` slot with `WARN`, the `f` variants format the message before emitting
through the same path, and `With` appends its pairs after `ts`.

The one new behavior to be aware of: `SetLevel` defaults to `LevelInfo`, which
matches v0 (everything v0 could emit was INFO or ERROR). Calling
`SetLevel(LevelWarn)` or higher will suppress `Info` calls that previously
always emitted.

## Run sample program

```bash
go run ./cmd/xlog-sample
```

## Run tests

Unit tests + coverage:

```bash
go test ./... -coverpkg=./... -coverprofile=cover.out
go tool cover -func=cover.out | tail -n 1
```

Race test:

```bash
go test ./... -race
```

Or use `./tests.sh` (enforces 100% coverage).
