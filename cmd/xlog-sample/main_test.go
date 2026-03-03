package main

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestMainEmitsLogs(t *testing.T) {
	// Capture stdout by redirecting os.Stdout to a pipe.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()

	main()

	_ = w.Close()

	sc := bufio.NewScanner(r)

	count := 0
	seenSample := false
	for sc.Scan() {
		count++
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("json: %v; line=%s", err, sc.Text())
		}
		if lvl, _ := m["level"].(string); lvl == "" {
			t.Fatalf("missing level; line=%s", sc.Text())
		}
		if msg, _ := m["msg"].(string); strings.Contains(msg, "samplepkg") {
			seenSample = true
		}
		if caller, _ := m["caller"].(string); caller == "" {
			t.Fatalf("missing caller; line=%s", sc.Text())
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}

	// Expected emitted logs:
	// main: Info + Error + Error2 (3)
	// samplepkg: Info + Error + Error2 (3)
	if count != 6 {
		t.Fatalf("expected 6 log lines, got %d", count)
	}
	if !seenSample {
		t.Fatalf("expected to see samplepkg logs in output")
	}
}
