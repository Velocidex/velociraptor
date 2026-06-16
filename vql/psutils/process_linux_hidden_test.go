//go:build linux
// +build linux

package psutils

import (
	"os"
	"strings"
	"testing"
)

func TestParseProcStatus(t *testing.T) {
	data := strings.Join([]string{
		"Name:\tsshd",
		"Umask:\t0022",
		"State:\tS (sleeping)",
		"Tgid:\t1234",
		"Ngid:\t0",
		"Pid:\t1234",
		"PPid:\t1",
		"TracerPid:\t0",
		"Uid:\t1000\t1000\t1000\t1000",
		"Gid:\t1000\t1000\t1000\t1000",
	}, "\n")

	name, ppid, uid := parseProcStatus([]byte(data))
	if name != "sshd" {
		t.Fatalf("Name: got %q, want %q", name, "sshd")
	}
	if ppid != 1 {
		t.Fatalf("Ppid: got %d, want 1", ppid)
	}
	if uid != 1000 {
		t.Fatalf("Uid: got %d, want 1000", uid)
	}
}

func TestParseProcStatusMissingFields(t *testing.T) {

	data := "Name:\tonly-name\n"
	name, ppid, uid := parseProcStatus([]byte(data))
	if name != "only-name" {
		t.Fatalf("Name: got %q", name)
	}
	if ppid != 0 {
		t.Fatalf("Ppid default should be 0, got %d", ppid)
	}
	if uid != -1 {
		t.Fatalf("Uid default should be -1, got %d", uid)
	}
}

func TestReadProcCreateTime_AwkwardComm(t *testing.T) {
	tmp := t.TempDir()
	statPath := tmp + "/stat"

	fields := make([]string, 22)
	fields[0] = "S"
	for i := 1; i < len(fields); i++ {
		fields[i] = "0"
	}
	fields[19] = "12345"
	content := "1 (weird ) name) " + strings.Join(fields, " ") + "\n"
	if err := os.WriteFile(statPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _ = readProcCreateTime(statPath)
}
