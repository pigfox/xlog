package xlog

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var global atomic.Pointer[slog.Logger]

// Init installs a thread-safe JSON logger writing to w (or stdout if nil).
//
// Output key order is fixed:
// {"caller":"...","level":"...","msg":"...","ts":"..."}
func Init(w io.Writer) {
	if w == nil {
		w = os.Stdout
	}
	global.Store(slog.New(newOrderedHandler(w)))
}

func get() *slog.Logger {
	if l := global.Load(); l != nil {
		return l
	}
	Init(os.Stdout)
	return global.Load()
}

// Info logs an informational message.
func Info(msg string) {
	get().InfoContext(context.Background(), msg, callerAttr())
}

// Error logs an error. No-op if err is nil.
func Error(err error) {
	if err == nil {
		return
	}
	get().ErrorContext(context.Background(), err.Error(), callerAttr())
}

// Error2 logs an error, prefixing msg with "prefix: " if prefix is not empty.
// No-op if err is nil.
func Error2(prefix string, err error) {
	if err == nil {
		return
	}
	msg := err.Error()
	if prefix != "" {
		msg = prefix + ": " + msg
	}
	get().ErrorContext(context.Background(), msg, callerAttr())
}

func callerAttr() slog.Attr {
	file, line := callerFileLine()
	return slog.String("caller", formatCaller(file, line))
}

func callerFileLine() (string, int) {
	pcs := make([]uintptr, 32)
	// Skip 0: runtime.Callers, 1: callerFileLine, 2: callerAttr, 3: Info/Error/Error2
	n := runtime.Callers(4, pcs)
	frames := runtime.CallersFrames(pcs[:n])

	for {
		fr, more := frames.Next()
		// Skip frames in this module's xlog package.
		if !strings.Contains(fr.Function, "/xlog.") && !strings.HasSuffix(fr.Function, ".xlog") {
			if fr.File != "" && fr.Line != 0 {
				return fr.File, fr.Line
			}
		}
		if !more {
			break
		}
	}
	return "unknown.go", 0
}

// formatCaller formats file+line as "pkgname.filename line N".
func formatCaller(file string, line int) string {
	filename := filepath.Base(file)
	pkg := filepath.Base(filepath.Dir(file))
	return pkg + "." + filename + " line " + strconv.Itoa(line)
}

// ---- ordered JSON handler ----

type orderedHandler struct {
	w  io.Writer
	mu sync.Mutex
}

func newOrderedHandler(w io.Writer) slog.Handler {
	return &orderedHandler{w: w}
}

func (h *orderedHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *orderedHandler) Handle(_ context.Context, r slog.Record) error {
	caller := ""
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "caller" {
			caller = a.Value.String()
			return false
		}
		return true
	})

	line := orderedLine{
		Caller: caller,
		Level:  r.Level.String(),
		Msg:    r.Message,
		Ts:     r.Time.Format(time.RFC3339Nano),
	}

	b, err := json.Marshal(line)
	if err != nil {
		return err
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err = h.w.Write(append(b, '\n'))
	return err
}

func (h *orderedHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *orderedHandler) WithGroup(_ string) slog.Handler     { return h }

type orderedLine struct {
	Caller string `json:"caller"`
	Level  string `json:"level"`
	Msg    string `json:"msg"`
	Ts     string `json:"ts"`
}
