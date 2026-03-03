package samplepkg

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pigfox/xlog"
)

func TestRunEmitsExpectedMessagesAndCaller(t *testing.T) {
	var buf bytes.Buffer
	xlog.Init(&buf)

	Run()

	lines := readLines(t, &buf)

	// We expect 3 lines: Info + Error + Error2 (nil error calls produce no output).
	if len(lines) != 3 {
		t.Fatalf("expected 3 log lines, got %d: %v", len(lines), lines)
	}

	// Line 1: INFO from samplepkg
	m1 := parseJSONLine(t, lines[0])
	if got := asString(m1["level"]); got != "INFO" {
		t.Fatalf("expected INFO, got %q", got)
	}
	if got := asString(m1["msg"]); got != "info from samplepkg" {
		t.Fatalf("unexpected msg: %q", got)
	}
	caller1 := asString(m1["caller"])
	if !strings.HasPrefix(caller1, "samplepkg.callingfile.go line ") {
		t.Fatalf("caller should point to samplepkg.callingfile.go, got %q", caller1)
	}

	// Line 2: ERROR plain error
	m2 := parseJSONLine(t, lines[1])
	if got := asString(m2["level"]); got != "ERROR" {
		t.Fatalf("expected ERROR, got %q", got)
	}
	if got := asString(m2["msg"]); got != "plain error from samplepkg" {
		t.Fatalf("unexpected msg: %q", got)
	}

	// Line 3: ERROR prefixed error
	m3 := parseJSONLine(t, lines[2])
	if got := asString(m3["level"]); got != "ERROR" {
		t.Fatalf("expected ERROR, got %q", got)
	}
	if got := asString(m3["msg"]); got != "samplepkg: prefixed error from samplepkg" {
		t.Fatalf("unexpected msg: %q", got)
	}
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
		t.Fatalf("json: %v; line=%s", err, line)
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
