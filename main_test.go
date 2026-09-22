package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The binary is built once and reused by all CLI tests.
var binPath = ""

func TestMain(m *testing.M) {
	var err error
	binPath, err = exec.LookPath("go")
	if err != nil {
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func runCLI(t *testing.T, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(binPath, append([]string{"run", "."}, args...)...)
	out, err := cmd.CombinedOutput()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("run: %v", err)
	}
	return string(out), code
}

func TestCLIValidate(t *testing.T) {
	path := t.TempDir() + "/spec.json"
	spec := `{"routes": [{"method": "GET", "path": "/x", "body": {}}]}`
	if err := os.WriteFile(path, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code := runCLI(t, path, "-validate")
	if code != 0 || !strings.Contains(out, "spec ok: 1 routes") {
		t.Fatalf("validate failed: code=%d out=%s", code, out)
	}

	broken := t.TempDir() + "/broken.json"
	if err := os.WriteFile(broken, []byte(`{"routes": []}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code = runCLI(t, broken, "-validate")
	if code != 1 {
		t.Fatalf("broken spec must exit 1: code=%d out=%s", code, out)
	}
}

func TestCLIUsageAndVersion(t *testing.T) {
	if _, code := runCLI(t, "-h"); code != 0 {
		t.Fatalf("-h must exit 0")
	}
	out, code := runCLI(t, "-V")
	if code != 0 || !strings.Contains(out, "api-mock 1.0.0") {
		t.Fatalf("version output: %s", out)
	}
	out, _ = runCLI(t, "a.json", "b.json")
	if !strings.Contains(out, "expected exactly one spec file") {
		t.Fatalf("two specs must be a usage error: %s", out)
	}
}
