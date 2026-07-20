package xlog

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// Expected values, kept as named consts so no assertion carries a magic literal.
const (
	keyCaller = "caller"
	keyLevel  = "level"
	keyMsg    = "msg"
	keyTs     = "ts"

	lvlDebug = "DEBUG"
	lvlInfo  = "INFO"
	lvlWarn  = "WARN"
	lvlError = "ERROR"

	msgHello   = "hello"
	msgBoom    = "boom"
	prefixDB   = "db"
	msgDBBoom  = prefixDB + ": " + msgBoom
	fmtCount   = "count=%d"
	fmtCounted = "count=7"
	argCount   = 7

	kvKey  = "user_id"
	kvVal  = 42
	kvKey2 = "route"
	kvVal2 = "/v1/x"

	callerRe = `\w+\.\w+\.go line \d+`
)

// ---- v0 golden format ----

// TestV0ByteCompatibility pins the exact line shape the four legacy functions
// have always produced. ts is variable, so it is matched structurally; every
// other byte is asserted literally.
func TestV0ByteCompatibility(t *testing.T) {
	const goldenRe = `^\{"caller":"` + callerRe + `","level":"(INFO|ERROR)","msg":"[^"]*","ts":"[^"]+"\}$`

	var buf bytes.Buffer
	Init(&buf)

	Info(msgHello)
	Error(errors.New(msgBoom))
	Error2(prefixDB, errors.New(msgBoom))

	lines := readLines(t, &buf)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), lines)
	}

	re := regexp.MustCompile(goldenRe)
	for _, l := range lines {
		if !re.MatchString(l) {
			t.Fatalf("line does not match v0 golden shape: %q", l)
		}
	}

	// Key order is part of the contract.
	assertKeyOrder(t, lines[0], keyCaller, keyLevel, keyMsg, keyTs)

	assertField(t, lines[0], keyLevel, lvlInfo)
	assertField(t, lines[0], keyMsg, msgHello)
	assertField(t, lines[1], keyLevel, lvlError)
	assertField(t, lines[1], keyMsg, msgBoom)
	assertField(t, lines[2], keyLevel, lvlError)
	assertField(t, lines[2], keyMsg, msgDBBoom)

	caller := asString(parseJSONLine(t, lines[0])[keyCaller])
	if strings.ContainsAny(caller, `/\`) {
		t.Fatalf("caller should not contain path separators, got %q", caller)
	}
}

// ---- package-level functions ----

func TestPackageFunctions(t *testing.T) {
	tests := []struct {
		name      string
		call      func()
		wantLevel string
		wantMsg   string
	}{
		{"Info", func() { Info(msgHello) }, lvlInfo, msgHello},
		{"Infof", func() { Infof(fmtCount, argCount) }, lvlInfo, fmtCounted},
		{"Warn", func() { Warn(msgHello) }, lvlWarn, msgHello},
		{"Warn2", func() { Warn2(prefixDB, errors.New(msgBoom)) }, lvlWarn, msgDBBoom},
		{"Warn2NoPrefix", func() { Warn2("", errors.New(msgBoom)) }, lvlWarn, msgBoom},
		{"Error", func() { Error(errors.New(msgBoom)) }, lvlError, msgBoom},
		{"Error2", func() { Error2(prefixDB, errors.New(msgBoom)) }, lvlError, msgDBBoom},
		{"Error2NoPrefix", func() { Error2("", errors.New(msgBoom)) }, lvlError, msgBoom},
		{"Errorf", func() { Errorf(fmtCount, argCount) }, lvlError, fmtCounted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			Init(&buf)
			tt.call()

			m := parseJSONLine(t, readOneLine(t, &buf))
			if got := asString(m[keyLevel]); got != tt.wantLevel {
				t.Fatalf("level: got %q, want %q", got, tt.wantLevel)
			}
			if got := asString(m[keyMsg]); got != tt.wantMsg {
				t.Fatalf("msg: got %q, want %q", got, tt.wantMsg)
			}
			if got := asString(m[keyCaller]); !regexp.MustCompile(callerRe).MatchString(got) {
				t.Fatalf("caller format unexpected: %q", got)
			}
		})
	}
}

func TestNilErrorsAreSilent(t *testing.T) {
	tests := []struct {
		name string
		call func()
	}{
		{"Error", func() { Error(nil) }},
		{"Error2", func() { Error2(prefixDB, nil) }},
		{"Warn2", func() { Warn2(prefixDB, nil) }},
		{"LoggerError", func() { With(kvKey, kvVal).Error(nil) }},
		{"LoggerError2", func() { With(kvKey, kvVal).Error2(prefixDB, nil) }},
		{"LoggerWarn2", func() { With(kvKey, kvVal).Warn2(prefixDB, nil) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			Init(&buf)
			tt.call()
			if buf.Len() != 0 {
				t.Fatalf("expected no output, got %q", buf.String())
			}
		})
	}
}

// ---- Logger / With ----

func TestLoggerMethods(t *testing.T) {
	tests := []struct {
		name      string
		call      func(*Logger)
		wantLevel string
		wantMsg   string
	}{
		{"Info", func(g *Logger) { g.Info(msgHello) }, lvlInfo, msgHello},
		{"Infof", func(g *Logger) { g.Infof(fmtCount, argCount) }, lvlInfo, fmtCounted},
		{"Warn", func(g *Logger) { g.Warn(msgHello) }, lvlWarn, msgHello},
		{"Warn2", func(g *Logger) { g.Warn2(prefixDB, errors.New(msgBoom)) }, lvlWarn, msgDBBoom},
		{"Error", func(g *Logger) { g.Error(errors.New(msgBoom)) }, lvlError, msgBoom},
		{"Error2", func(g *Logger) { g.Error2(prefixDB, errors.New(msgBoom)) }, lvlError, msgDBBoom},
		{"Errorf", func(g *Logger) { g.Errorf(fmtCount, argCount) }, lvlError, fmtCounted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			Init(&buf)
			tt.call(With(kvKey, kvVal))

			line := readOneLine(t, &buf)
			m := parseJSONLine(t, line)
			if got := asString(m[keyLevel]); got != tt.wantLevel {
				t.Fatalf("level: got %q, want %q", got, tt.wantLevel)
			}
			if got := asString(m[keyMsg]); got != tt.wantMsg {
				t.Fatalf("msg: got %q, want %q", got, tt.wantMsg)
			}
			if got, ok := m[kvKey].(float64); !ok || int(got) != kvVal {
				t.Fatalf("expected %s=%d, got %v (line=%s)", kvKey, kvVal, m[kvKey], line)
			}
			// Envelope keys stay first and in order.
			assertKeyOrder(t, line, keyCaller, keyLevel, keyMsg, keyTs, kvKey)
		})
	}
}

func TestWithChaining(t *testing.T) {
	var buf bytes.Buffer
	Init(&buf)

	With(kvKey, kvVal).With(kvKey2, kvVal2).Info(msgHello)

	line := readOneLine(t, &buf)
	m := parseJSONLine(t, line)
	if got, ok := m[kvKey].(float64); !ok || int(got) != kvVal {
		t.Fatalf("expected %s=%d, got %v", kvKey, kvVal, m[kvKey])
	}
	if got := asString(m[kvKey2]); got != kvVal2 {
		t.Fatalf("expected %s=%q, got %q", kvKey2, kvVal2, got)
	}
	assertKeyOrder(t, line, keyCaller, keyLevel, keyMsg, keyTs, kvKey, kvKey2)
}

// TestWithNoAttrsReusesHandler covers the empty-attrs short circuit in
// WithAttrs; output must stay byte-shaped like v0.
func TestWithNoAttrsReusesHandler(t *testing.T) {
	var buf bytes.Buffer
	Init(&buf)

	With().Info(msgHello)

	line := readOneLine(t, &buf)
	assertKeyOrder(t, line, keyCaller, keyLevel, keyMsg, keyTs)
	m := parseJSONLine(t, line)
	if len(m) != 4 {
		t.Fatalf("expected exactly 4 keys, got %d: %v", len(m), m)
	}
}

func TestLoggerCallerPointsAtCallSite(t *testing.T) {
	var buf bytes.Buffer
	Init(&buf)

	Info(msgHello)
	With(kvKey, kvVal).Info(msgHello)

	lines := readLines(t, &buf)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	direct := asString(parseJSONLine(t, lines[0])[keyCaller])
	viaLogger := asString(parseJSONLine(t, lines[1])[keyCaller])
	// Both resolve past the xlog package to the same non-xlog frame.
	if direct == "" || viaLogger == "" {
		t.Fatalf("empty caller: direct=%q logger=%q", direct, viaLogger)
	}
	if strings.Split(direct, " line ")[0] != strings.Split(viaLogger, " line ")[0] {
		t.Fatalf("Logger caller resolved to a different file: %q vs %q", direct, viaLogger)
	}
}

// ---- levels ----

func TestSetLevelFiltering(t *testing.T) {
	defer SetLevel(LevelInfo)

	tests := []struct {
		name     string
		level    Level
		call     func()
		wantLine bool
	}{
		{"InfoSuppressedAtError", LevelError, func() { Info(msgHello) }, false},
		{"WarnSuppressedAtError", LevelError, func() { Warn(msgHello) }, false},
		{"ErrorPassesAtError", LevelError, func() { Error(errors.New(msgBoom)) }, true},
		{"InfoSuppressedAtWarn", LevelWarn, func() { Info(msgHello) }, false},
		{"WarnPassesAtWarn", LevelWarn, func() { Warn(msgHello) }, true},
		{"InfoPassesAtInfo", LevelInfo, func() { Info(msgHello) }, true},
		{"InfoPassesAtDebug", LevelDebug, func() { Info(msgHello) }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			Init(&buf)
			SetLevel(tt.level)
			tt.call()

			if got := buf.Len() > 0; got != tt.wantLine {
				t.Fatalf("emitted=%v, want %v (output=%q)", got, tt.wantLine, buf.String())
			}
		})
	}
}

func TestSetLevelReenables(t *testing.T) {
	defer SetLevel(LevelInfo)

	var buf bytes.Buffer
	Init(&buf)

	SetLevel(LevelError)
	Info(msgHello)
	if buf.Len() != 0 {
		t.Fatalf("expected suppression, got %q", buf.String())
	}

	SetLevel(LevelInfo)
	Info(msgHello)

	m := parseJSONLine(t, readOneLine(t, &buf))
	if got := asString(m[keyLevel]); got != lvlInfo {
		t.Fatalf("level: got %q, want %q", got, lvlInfo)
	}
}

func TestLevelConstsMatchSlog(t *testing.T) {
	tests := []struct {
		level Level
		want  string
	}{
		{LevelDebug, lvlDebug},
		{LevelInfo, lvlInfo},
		{LevelWarn, lvlWarn},
		{LevelError, lvlError},
	}
	for _, tt := range tests {
		if got := tt.level.String(); got != tt.want {
			t.Fatalf("level %v: got %q, want %q", tt.level, got, tt.want)
		}
	}
}

// ---- init / default writer ----

func TestInitNilWriterDefaultsToStdout(t *testing.T) {
	defer func() { global.Store(nil) }()

	Init(nil)
	if global.Load() == nil {
		t.Fatal("expected a logger after Init(nil)")
	}
}

// TestLazyDefaultInit covers the path where get() is reached before Init.
func TestLazyDefaultInit(t *testing.T) {
	global.Store(nil)
	defer func() { global.Store(nil) }()

	if got := get(); got == nil {
		t.Fatal("get() should lazily install a default logger")
	}
	if global.Load() == nil {
		t.Fatal("lazy Init should have stored the logger")
	}
}

// ---- handler internals ----

func TestFormatCaller(t *testing.T) {
	tests := []struct {
		file string
		line int
		want string
	}{
		{"/a/b/c/pkg/file.go", 123, "pkg.file.go line 123"},
		{"/x/main.go", 1, "x.main.go line 1"},
	}
	for _, tt := range tests {
		if got := formatCaller(tt.file, tt.line); got != tt.want {
			t.Fatalf("formatCaller(%q, %d) = %q, want %q", tt.file, tt.line, got, tt.want)
		}
	}
}

// TestFirstNonXlogFrameFallback covers the exhausted-frames path.
func TestFirstNonXlogFrameFallback(t *testing.T) {
	const wantFile, wantLine = "unknown.go", 0

	file, line := firstNonXlogFrame(nil)
	if file != wantFile || line != wantLine {
		t.Fatalf("got (%q, %d), want (%q, %d)", file, line, wantFile, wantLine)
	}
}

func TestQuoteKey(t *testing.T) {
	const want = `"` + kvKey + `"`
	if got := string(quoteKey(kvKey)); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestHandleAttrMarshalError covers the encode failure path via a value the
// JSON encoder cannot represent.
func TestHandleAttrMarshalError(t *testing.T) {
	var buf bytes.Buffer
	Init(&buf)

	With(kvKey, make(chan int)).Info(msgHello)

	if buf.Len() != 0 {
		t.Fatalf("expected no output on encode failure, got %q", buf.String())
	}
}

// failingWriter always reports an error, covering the write error path.
type failingWriter struct{}

var errWrite = errors.New("write failed")

func (failingWriter) Write([]byte) (int, error) { return 0, errWrite }

func TestHandleWriteError(t *testing.T) {
	Init(failingWriter{})
	defer global.Store(nil)

	// slog discards the handler error; the assertion is that it does not panic.
	Info(msgHello)
}

// TestWithAttrsEmptyReusesHandler covers the short circuit; slog.Logger.With()
// with no arguments never reaches the handler, so it is exercised directly.
func TestWithAttrsEmptyReusesHandler(t *testing.T) {
	h := newOrderedHandler(&bytes.Buffer{})
	if got := h.WithAttrs(nil); got != h {
		t.Fatal("WithAttrs(nil) should return the same handler")
	}
}

func TestWithGroupIsNoop(t *testing.T) {
	h := newOrderedHandler(&bytes.Buffer{})
	if got := h.WithGroup("g"); got != h {
		t.Fatal("WithGroup should return the same handler")
	}
}

func TestEnabledRespectsLevel(t *testing.T) {
	defer SetLevel(LevelInfo)

	h := newOrderedHandler(&bytes.Buffer{})
	SetLevel(LevelWarn)

	if h.Enabled(t.Context(), LevelInfo) {
		t.Fatal("Info should be disabled at Warn")
	}
	if !h.Enabled(t.Context(), LevelError) {
		t.Fatal("Error should be enabled at Warn")
	}
}

func TestEncodeLineOrder(t *testing.T) {
	const want = `{"a":"1","b":2}`

	got, err := encodeLine([]slog.Attr{slog.String("a", "1"), slog.Int("b", 2)})
	if err != nil {
		t.Fatalf("encodeLine: %v", err)
	}
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// ---- concurrency ----

func TestConcurrentUse(t *testing.T) {
	defer SetLevel(LevelInfo)

	const goroutines, iterations = 8, 50

	var mu sync.Mutex
	Init(&lockedBuffer{mu: &mu})

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			logger := With(kvKey, i)
			for j := 0; j < iterations; j++ {
				Info(msgHello)
				Error(errors.New(msgBoom))
				logger.Warn(msgHello)
				SetLevel(LevelInfo)
			}
		}(i)
	}
	wg.Wait()
}

// lockedBuffer is a writer that is safe to inspect from the test goroutine.
type lockedBuffer struct {
	mu  *sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// ---- helpers ----

func readOneLine(t *testing.T, buf *bytes.Buffer) string {
	t.Helper()
	sc := bufio.NewScanner(bytes.NewReader(buf.Bytes()))
	if !sc.Scan() {
		t.Fatalf("expected one line, got none")
	}
	return sc.Text()
}

func readLines(t *testing.T, buf *bytes.Buffer) []string {
	t.Helper()
	sc := bufio.NewScanner(bytes.NewReader(buf.Bytes()))
	var out []string
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	return out
}

func parseJSONLine(t *testing.T, line string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("failed to parse json: %v; line=%s", err, line)
	}
	return m
}

// assertKeyOrder checks that the given keys appear in order in the raw line.
func assertKeyOrder(t *testing.T, line string, keys ...string) {
	t.Helper()
	pos := -1
	for _, k := range keys {
		i := strings.Index(line, `"`+k+`":`)
		if i < 0 {
			t.Fatalf("missing key %q in line=%s", k, line)
		}
		if i <= pos {
			t.Fatalf("key %q out of order at %d (after %d); line=%s", k, i, pos, line)
		}
		pos = i
	}
}

func assertField(t *testing.T, line, key, want string) {
	t.Helper()
	if got := asString(parseJSONLine(t, line)[key]); got != want {
		t.Fatalf("%s: got %q, want %q; line=%s", key, got, want, line)
	}
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
