# xlog

Thread-safe JSON logger with a short global API.

## Install

```bash
go get github.com/pigfox/xlog@latest
```

## Initialize

Call `Init()` once at process startup.

```go
package main

import (
	"os"

	"github.com/pigfox/xlog"
)

func main() {
	xlog.Init(os.Stdout)
	xlog.Info("server starting")
}
```

## API

- `xlog.Init(w io.Writer)` (nil -> stdout)
- `xlog.Info(msg string)`
- `xlog.Error(err error)` (no-op if err is nil)
- `xlog.Error2(prefix string, err error)` (prefix + ": " + err)

## Output format

Keys are emitted in this fixed order:

```json
{"caller":"pkg.file.go line 123","level":"ERROR","msg":"something failed","ts":"2026-03-03T10:50:25.389260945-08:00"}
```

Caller behavior:

- When called from `main`, `caller` points at your `main.go`.
- When called from another package, `caller` points at the calling package’s file/line.

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

Or use `./tests.sh`
