// Package xlog is a thread-safe JSON logger with a short global API.
//
// v1 output is byte-compatible with v0 for Init/Info/Error/Error2. New
// functions extend the same line format: the four envelope keys always come
// first, in order, and With attributes are appended after them.
package xlog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

// Level is the severity type. It aliases slog.Level so callers may pass
// slog levels interchangeably.
type Level = slog.Level

// Severity levels accepted by SetLevel.
const (
	LevelDebug = slog.LevelDebug
	LevelInfo  = slog.LevelInfo
	LevelWarn  = slog.LevelWarn
	LevelError = slog.LevelError
)

var (
	global   atomic.Pointer[slog.Logger]
	levelVar = new(slog.LevelVar) // safe for concurrent use
)

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

// SetLevel sets the minimum severity that will be emitted. It may be called
// concurrently with logging.
func SetLevel(l Level) {
	levelVar.Set(l)
}

func get() *slog.Logger {
	if l := global.Load(); l != nil {
		return l
	}
	Init(os.Stdout)
	return global.Load()
}

// emit writes msg at lvl, attaching the caller of the exported entry point.
func emit(l *slog.Logger, lvl Level, msg string) {
	l.LogAttrs(context.Background(), lvl, msg, callerAttr())
}

// prefixed joins prefix and err.Error() the way Error2 has always done.
func prefixed(prefix string, err error) string {
	msg := err.Error()
	if prefix != "" {
		msg = prefix + ": " + msg
	}
	return msg
}

// Info logs an informational message.
func Info(msg string) { emit(get(), LevelInfo, msg) }

// Infof logs a formatted informational message.
func Infof(format string, a ...any) { emit(get(), LevelInfo, fmt.Sprintf(format, a...)) }

// Warn logs a warning message.
func Warn(msg string) { emit(get(), LevelWarn, msg) }

// Warn2 logs a warning, prefixing msg with "prefix: " if prefix is not empty.
// No-op if err is nil.
func Warn2(prefix string, err error) {
	if err == nil {
		return
	}
	emit(get(), LevelWarn, prefixed(prefix, err))
}

// Error logs an error. No-op if err is nil.
func Error(err error) {
	if err == nil {
		return
	}
	emit(get(), LevelError, err.Error())
}

// Error2 logs an error, prefixing msg with "prefix: " if prefix is not empty.
// No-op if err is nil.
func Error2(prefix string, err error) {
	if err == nil {
		return
	}
	emit(get(), LevelError, prefixed(prefix, err))
}

// Errorf logs a formatted error message.
func Errorf(format string, a ...any) { emit(get(), LevelError, fmt.Sprintf(format, a...)) }

// ---- structured child logger ----

// Logger carries attributes that are appended to every line it emits, after
// the four envelope keys.
type Logger struct {
	l *slog.Logger
}

// With returns a Logger that appends the given key/value pairs to each line.
func With(kv ...any) *Logger {
	return &Logger{l: get().With(kv...)}
}

// With returns a Logger with additional key/value pairs appended.
func (g *Logger) With(kv ...any) *Logger { return &Logger{l: g.l.With(kv...)} }

// Info logs an informational message.
func (g *Logger) Info(msg string) { emit(g.l, LevelInfo, msg) }

// Infof logs a formatted informational message.
func (g *Logger) Infof(format string, a ...any) { emit(g.l, LevelInfo, fmt.Sprintf(format, a...)) }

// Warn logs a warning message.
func (g *Logger) Warn(msg string) { emit(g.l, LevelWarn, msg) }

// Warn2 logs a warning with an optional prefix. No-op if err is nil.
func (g *Logger) Warn2(prefix string, err error) {
	if err == nil {
		return
	}
	emit(g.l, LevelWarn, prefixed(prefix, err))
}

// Error logs an error. No-op if err is nil.
func (g *Logger) Error(err error) {
	if err == nil {
		return
	}
	emit(g.l, LevelError, err.Error())
}

// Error2 logs an error with an optional prefix. No-op if err is nil.
func (g *Logger) Error2(prefix string, err error) {
	if err == nil {
		return
	}
	emit(g.l, LevelError, prefixed(prefix, err))
}

// Errorf logs a formatted error message.
func (g *Logger) Errorf(format string, a ...any) { emit(g.l, LevelError, fmt.Sprintf(format, a...)) }

// ---- caller resolution ----

// callerKey is the envelope key holding the resolved call site.
const callerKey = "caller"

func callerAttr() slog.Attr {
	file, line := callerFileLine()
	return slog.String(callerKey, formatCaller(file, line))
}

func callerFileLine() (string, int) {
	pcs := make([]uintptr, 32)
	// Skip 0: runtime.Callers, 1: callerFileLine, 2: callerAttr, 3: emit.
	// Remaining xlog frames are dropped by name below, so wrapper methods on
	// Logger resolve to the same caller as the package-level functions.
	n := runtime.Callers(4, pcs)
	return firstNonXlogFrame(pcs[:n])
}

// firstNonXlogFrame returns the first frame outside this package.
func firstNonXlogFrame(pcs []uintptr) (string, int) {
	frames := runtime.CallersFrames(pcs)
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

// sink is shared by a handler and every handler derived from it via
// WithAttrs, so all of them serialize writes through one mutex.
type sink struct {
	w  io.Writer
	mu sync.Mutex
}

type orderedHandler struct {
	sink  *sink
	attrs []slog.Attr
}

func newOrderedHandler(w io.Writer) slog.Handler {
	return &orderedHandler{sink: &sink{w: w}}
}

func (h *orderedHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= levelVar.Level()
}

func (h *orderedHandler) Handle(_ context.Context, r slog.Record) error {
	caller := ""
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == callerKey {
			caller = a.Value.String()
		}
		return true
	})

	// The four envelope keys always come first, in this order. With
	// attributes follow, so lines carrying none are byte-identical to v0.
	attrs := make([]slog.Attr, 0, 4+len(h.attrs))
	attrs = append(attrs,
		slog.String(callerKey, caller),
		slog.String("level", r.Level.String()),
		slog.String("msg", r.Message),
		slog.String("ts", r.Time.Format(time.RFC3339Nano)),
	)
	attrs = append(attrs, h.attrs...)

	b, err := encodeLine(attrs)
	if err != nil {
		return err
	}

	h.sink.mu.Lock()
	defer h.sink.mu.Unlock()
	_, err = h.sink.w.Write(append(b, '\n'))
	return err
}

// encodeLine renders attrs as a JSON object, preserving their order.
func encodeLine(attrs []slog.Attr) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, a := range attrs {
		if i > 0 {
			buf.WriteByte(',')
		}
		vb, err := json.Marshal(a.Value.Any())
		if err != nil {
			return nil, err
		}
		buf.Write(quoteKey(a.Key))
		buf.WriteByte(':')
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// quoteKey JSON-quotes an object key. Marshalling a string cannot fail, so
// the error is discarded.
func quoteKey(k string) []byte {
	b, _ := json.Marshal(k)
	return b
}

func (h *orderedHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	merged := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	merged = append(merged, h.attrs...)
	merged = append(merged, attrs...)
	return &orderedHandler{sink: h.sink, attrs: merged}
}

func (h *orderedHandler) WithGroup(_ string) slog.Handler { return h }
