package xlog

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
)

func TestInitWritesJSONAndOrder(t *testing.T) {
	var buf bytes.Buffer
	Init(&buf)

	Info("hello")

	line := readOneLine(t, &buf)

	wantOrder := []string{`"caller":`, `"level":`, `"msg":`, `"ts":`}
	pos := -1
	for _, k := range wantOrder {
		i := strings.Index(line, k)
		if i < 0 {
			t.Fatalf("missing key %s in line=%s", k, line)
		}
		if i <= pos {
			t.Fatalf("key order wrong; %s at %d after %d; line=%s", k, i, pos, line)
		}
		pos = i
	}

	m := parseJSONLine(t, line)

	if got := asString(m["level"]); got != "INFO" {
		t.Fatalf("expected level INFO, got %q (line=%s)", got, line)
	}
	if got := asString(m["msg"]); got != "hello" {
		t.Fatalf("expected msg hello, got %q (line=%s)", got, line)
	}
	caller := asString(m["caller"])
	if caller == "" {
		t.Fatalf("expected caller, got empty (line=%s)", line)
	}
	if strings.Contains(caller, "/") || strings.Contains(caller, "\\") {
		t.Fatalf("caller should not contain path separators, got %q", caller)
	}
	if !regexp.MustCompile(`\w+\.\w+\.go line \d+`).MatchString(caller) {
		t.Fatalf("caller format unexpected: %q", caller)
	}
	if _, ok := m["ts"]; !ok {
		t.Fatalf("expected ts key, got: %v", m)
	}
}

func TestErrorNoopOnNil(t *testing.T) {
	var buf bytes.Buffer
	Init(&buf)

	Error(nil)

	if buf.Len() != 0 {
		t.Fatalf("expected no output, got: %q", buf.String())
	}
}

func TestErrorLogsErrorLevel(t *testing.T) {
	var buf bytes.Buffer
	Init(&buf)

	Error(errors.New("db connect failed"))

	line := readOneLine(t, &buf)
	m := parseJSONLine(t, line)

	if got := asString(m["level"]); got != "ERROR" {
		t.Fatalf("expected level ERROR, got %q (line=%s)", got, line)
	}
	if got := asString(m["msg"]); got != "db connect failed" {
		t.Fatalf("expected msg to be error string, got %q (line=%s)", got, line)
	}
	if _, ok := m["caller"]; !ok {
		t.Fatalf("expected caller key, got: %v", m)
	}
}

func TestError2PrefixesMsg(t *testing.T) {
	var buf bytes.Buffer
	Init(&buf)

	Error2("db", errors.New("connect failed"))

	line := readOneLine(t, &buf)
	m := parseJSONLine(t, line)

	if got := asString(m["level"]); got != "ERROR" {
		t.Fatalf("expected level ERROR, got %q (line=%s)", got, line)
	}
	if got := asString(m["msg"]); got != "db: connect failed" {
		t.Fatalf("expected prefixed msg, got %q (line=%s)", got, line)
	}
}

func TestFormatCaller(t *testing.T) {
	got := formatCaller("/a/b/c/pkg/file.go", 123)
	if got != "pkg.file.go line 123" {
		t.Fatalf("unexpected: %q", got)
	}
}

func readOneLine(t *testing.T, buf *bytes.Buffer) string {
	t.Helper()
	sc := bufio.NewScanner(bytes.NewReader(buf.Bytes()))
	if !sc.Scan() {
		t.Fatalf("expected one line, got none")
	}
	return sc.Text()
}

func parseJSONLine(t *testing.T, line string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("failed to parse json: %v; line=%s", err, line)
	}
	return m
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
