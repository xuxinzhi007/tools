package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 正常路径：内容落盘、权限正确、不留临时文件。
func TestWriteFileAtomicWritesContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "system_tags.json")

	if err := writeFileAtomic(path, []byte(`{"a":1}`), 0o644); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != `{"a":1}` {
		t.Fatalf("content = %q, want %q", got, `{"a":1}`)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// CreateTemp 建出来是 0600，必须显式 Chmod，否则权限悄悄变了
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Fatalf("perm = %o, want 644", perm)
	}

	assertNoTempLeftovers(t, dir)
}

// 覆盖已有文件：旧内容必须被完整替换，不能是新旧拼接。
func TestWriteFileAtomicOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "system_tags.json")
	if err := os.WriteFile(path, []byte("old-content-that-is-long"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := writeFileAtomic(path, []byte("new"), 0o644); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Fatalf("content = %q, want %q", got, "new")
	}
	assertNoTempLeftovers(t, dir)
}

// 核心承诺：写入失败时，原文件一个字都不能变。
// 用「目标目录只读」制造失败——CreateTemp 会先失败，原文件不受影响。
func TestWriteFileAtomicFailureKeepsOriginal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root 无视目录权限，这个用例构造不出失败")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "system_tags.json")
	original := `{"keep":"me"}`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	err := writeFileAtomic(path, []byte("should-not-land"), 0o644)
	if err == nil {
		t.Fatal("writeFileAtomic 应当在只读目录上失败，但返回了 nil")
	}

	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("原文件读不回来了: %v", readErr)
	}
	if string(got) != original {
		t.Fatalf("失败后原文件被改动了: %q, want %q", got, original)
	}
}

// 用旧实现（直接 WriteFile）会碎掉的场景，留给未来回归：
// 目标已存在但新内容更短时，非原子写会留下尾部残留。
func TestWriteFileAtomicShorterContentNoTail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.json")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 1000)), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := writeFileAtomic(path, []byte("short"), 0o644); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "short" {
		t.Fatalf("content = %q (len %d), want %q", got, len(got), "short")
	}
}

func assertNoTempLeftovers(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Fatalf("临时文件没清掉: %s", e.Name())
		}
	}
}
