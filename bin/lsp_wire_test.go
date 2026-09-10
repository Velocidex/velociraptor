package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// extractSection parses one delimited dump and returns its header,
// declared sizes and the raw section bytes. This mirrors how
// machine consumers are expected to read the log: use the byte
// counts from the header rather than line based parsing.
func extractSection(t *testing.T, data []byte) (
	shown, total int, header string, section string) {
	t.Helper()

	s := string(data)
	start := strings.Index(s, "==== wire")
	if start < 0 {
		t.Fatal("missing start marker")
	}
	line_term := strings.Index(s[start:], " ====\n")
	if line_term < 0 {
		t.Fatal("malformed start marker")
	}

	// Absolute offset of the first byte after the marker line.
	// Note len(s[start:section_start]) != section_start since the
	// marker does not start at 0.
	section_start := start + line_term + len(" ====\n")
	header = s[start:section_start]

	open := strings.Index(header, "(")
	if open < 0 {
		t.Fatalf("no size declaration in header: %q", header)
	}
	_, err := fmt.Sscanf(header[open:], "(%d/%d", &shown, &total)
	if err != nil {
		t.Fatalf("cant parse sizes from %q: %v", header[open:], err)
	}

	if len(s) < section_start+shown {
		t.Fatalf("section truncated on disk: want %v bytes", shown)
	}
	section = s[section_start : section_start+shown]
	return
}

// A frame larger than maxWireLogPayload must be written with a full
// size header but with the payload itself truncated.
func TestLoggingConnTruncates(t *testing.T) {
	dir := t.TempDir()
	log_path := filepath.Join(dir, "wire.log")
	log_file, err := os.Create(log_path)
	if err != nil {
		t.Fatal(err)
	}
	defer log_file.Close()

	conn := &loggingConn{log_file: log_file}
	payload := bytes.Repeat([]byte("A"), maxWireLogPayload+1000)
	conn.dump("---->", payload)

	data, err := os.ReadFile(log_path)
	if err != nil {
		t.Fatal(err)
	}

	shown, total, _, section := extractSection(t, data)

	if total != len(payload) || shown != maxWireLogPayload {
		t.Fatalf("wrong sizes: got %v/%v want %v/%v",
			shown, total, maxWireLogPayload, len(payload))
	}
	if section != string(payload[:maxWireLogPayload]) {
		t.Fatal("section contents differ from payload prefix")
	}
	if !strings.HasPrefix(
		string(data)[len(string(data))-len("\n==== end ====\n"):],
		"\n==== end ====\n") {
		t.Fatal("missing end delimiter")
	}
}

// Frames under the cap are written verbatim: the section contains
// exactly the payload bytes declared in the header.
func TestLoggingConnPassThrough(t *testing.T) {
	dir := t.TempDir()
	log_path := filepath.Join(dir, "wire.log")
	log_file, err := os.Create(log_path)
	if err != nil {
		t.Fatal(err)
	}
	defer log_file.Close()

	conn := &loggingConn{log_file: log_file}
	payload := []byte(`{"jsonrpc": "2.0"}`)
	conn.dump("<----", payload)

	data, err := os.ReadFile(log_path)
	if err != nil {
		t.Fatal(err)
	}

	shown, total, _, section := extractSection(t, data)

	if shown != len(payload) || total != len(payload) {
		t.Fatalf("wrong sizes: got %v/%v want %v",
			shown, total, len(payload))
	}
	if section != string(payload) {
		t.Fatalf("section mismatch: got %q want %q",
			section, string(payload))
	}
}
