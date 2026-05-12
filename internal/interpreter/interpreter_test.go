package interpreter

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestParseJSONOutputSequentialObjects(t *testing.T) {
	raw := `{
  "array a_read": 0,
  "array a_write": 128,
  "array b_read": 128,
  "array b_write": 0,
  "cacheBlockSize": 64,
  "cacheSize": 32768,
  "hit_read": 4993,
  "hit_write": 1920,
  "level_name": "L1",
  "miss_read": 128,
  "miss_write": 129,
  "way": 8
}
{
  "array a_read": 0,
  "array a_write": 128,
  "array b_read": 128,
  "array b_write": 0,
  "cacheBlockSize": 64,
  "cacheSize": 262144,
  "hit_read": 0,
  "hit_write": 0,
  "level_name": "L2",
  "miss_read": 128,
  "miss_write": 129,
  "way": 8
}`

	result, err := parseOutput(raw)
	if err != nil {
		t.Fatalf("parseOutput returned error: %v", err)
	}

	if result.L1.CacheLevel != "L1" || result.L1.CacheSizeKB != 32 {
		t.Fatalf("L1 summary = %+v", result.L1)
	}
	if result.L2.CacheLevel != "L2" || result.L2.CacheSizeKB != 256 {
		t.Fatalf("L2 summary = %+v", result.L2)
	}
	if result.L1.TotalAccesses != 7170 || result.L1.TotalMisses != 257 {
		t.Fatalf("L1 totals = access %d misses %d", result.L1.TotalAccesses, result.L1.TotalMisses)
	}
	if result.MemoryReads != 128 || result.MemoryWrites != 129 {
		t.Fatalf("memory metrics = reads %d writes %d", result.MemoryReads, result.MemoryWrites)
	}
	if len(result.Arrays) != 4 {
		t.Fatalf("len(Arrays) = %d, want 4: %+v", len(result.Arrays), result.Arrays)
	}

	want := map[string]uint64{
		"L1/a": 128,
		"L1/b": 128,
		"L2/a": 128,
		"L2/b": 128,
	}
	for _, metric := range result.Arrays {
		key := metric.CacheLevel + "/" + metric.ArrayName
		if metric.MissesTotal != want[key] {
			t.Fatalf("%s misses_total = %d, want %d", key, metric.MissesTotal, want[key])
		}
	}
}

func TestParseJSONOutputAfterPrefix(t *testing.T) {
	raw := `cats started
{"level_name":"l1","cacheSize":32,"cacheBlockSize":64,"way":8,"hit_read":1,"hit_write":2,"miss_read":3,"miss_write":4,"array x_read":3,"array x_write":4}`

	result, err := parseOutput(raw)
	if err != nil {
		t.Fatalf("parseOutput returned error: %v", err)
	}
	if result.L1.CacheLevel != "L1" || result.L1.CacheSizeKB != 32 {
		t.Fatalf("L1 summary = %+v", result.L1)
	}
}

func TestResultFilePath(t *testing.T) {
	got := resultFilePath("/tmp/source/test.c")
	want := "/tmp/source/test_result"
	if got != want {
		t.Fatalf("resultFilePath() = %q, want %q", got, want)
	}
}

func TestRunPrefersResultFile(t *testing.T) {
	workDir := t.TempDir()
	sourcePath := filepath.Join(workDir, "sample.c")
	if err := os.WriteFile(sourcePath, []byte("int main(){return 0;}"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	scriptPath := filepath.Join(workDir, "fake-cats.sh")
	script := "#!/bin/sh\ncat <<'EOF' > sample_result\n{\"level_name\":\"L1\",\"cacheSize\":32768,\"cacheBlockSize\":64,\"way\":8,\"hit_read\":10,\"hit_write\":20,\"miss_read\":1,\"miss_write\":2,\"array a_read\":1,\"array a_write\":2}\n{\"level_name\":\"L2\",\"cacheSize\":262144,\"cacheBlockSize\":64,\"way\":8,\"hit_read\":5,\"hit_write\":6,\"miss_read\":7,\"miss_write\":8,\"array a_read\":7,\"array a_write\":8}\nEOF\necho ignored stdout\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake interpreter: %v", err)
	}

	interp := New(scriptPath, 5)
	result, err := interp.Run(context.Background(), sourcePath)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.L1.TotalMisses != 3 || result.L2.TotalMisses != 15 {
		t.Fatalf("unexpected parsed result: %+v", result)
	}
	if result.MemoryReads != 7 || result.MemoryWrites != 8 {
		t.Fatalf("unexpected memory metrics: %+v", result)
	}
}

func TestRunFallsBackToSingleResultFile(t *testing.T) {
	workDir := t.TempDir()
	sourcePath := filepath.Join(workDir, "12345678-1234-1234-1234-1234567890ab.c")
	if err := os.WriteFile(sourcePath, []byte("int main(){return 0;}"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	scriptPath := filepath.Join(workDir, "fake-cats.sh")
	script := "#!/bin/sh\ncat <<'EOF' > short_result\n{\"level_name\":\"L1\",\"cacheSize\":32768,\"cacheBlockSize\":64,\"way\":8,\"hit_read\":10,\"hit_write\":20,\"miss_read\":1,\"miss_write\":2}\n{\"level_name\":\"L2\",\"cacheSize\":262144,\"cacheBlockSize\":64,\"way\":8,\"hit_read\":5,\"hit_write\":6,\"miss_read\":7,\"miss_write\":8}\nEOF\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake interpreter: %v", err)
	}

	interp := New(scriptPath, 5)
	result, err := interp.Run(context.Background(), sourcePath)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.L1.CacheLevel != "L1" || result.L2.CacheLevel != "L2" {
		t.Fatalf("unexpected parsed result: %+v", result)
	}
}
