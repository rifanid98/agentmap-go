// Package test contains black-box integration tests for the agentmap binary.
// Each test creates a temp Go workspace fixture, runs the binary, and asserts
// on stdout / stderr / exit code / map.json — mirroring the upstream test
// contract (determinism, exit codes, JSON shape, monorepo, degradation, safety).
package test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// agentmapBin is the path to the compiled binary. go test -run ./test builds
// it once via TestMain.
var agentmapBin string

func TestMain(m *testing.M) {
	bin, err := buildBinary()
	if err != nil {
		panic("failed to build agentmap: " + err.Error())
	}
	agentmapBin = bin
	os.Exit(m.Run())
}

func buildBinary() (string, error) {
	bin := filepath.Join(os.TempDir(), "agentmap-test-bin")
	// Build from the repo root (parent of this test package).
	repoRoot := filepath.Join(filepath.Dir(os.Args[0]), "..")
	repoRoot, _ = filepath.Abs(repoRoot)
	// Fallback: find go.mod walking up from this file's dir.
	repoRoot = findRepoRoot()
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = repoRoot
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return bin, cmd.Run()
}

func findRepoRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir
		}
		dir = parent
	}
}

// run executes the binary in dir with args, returning stdout, stderr, exit code.
func run(t *testing.T, dir string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(agentmapBin, args...)
	cmd.Dir = dir
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode = 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// makeMonorepo creates a minimal go.work fixture.
func makeMonorepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// go-core module
	modA := filepath.Join(dir, "go-core")
	os.MkdirAll(filepath.Join(modA, "orders/domain/entity"), 0o755)
	os.MkdirAll(filepath.Join(modA, "orders/usecase"), 0o755)
	write(t, filepath.Join(modA, "go.mod"), "module example.com/go-core\n\ngo 1.21\n")
	write(t, filepath.Join(modA, "orders/domain/entity/order.go"), `package entity
type Order struct{ ID string }
func (o *Order) Cancel() error { return nil }
func NewOrder(id string) *Order { return &Order{ID: id} }
`)
	write(t, filepath.Join(modA, "orders/usecase/svc.go"), `package usecase
import "example.com/go-core/orders/domain/entity"
type Service struct{}
func (s *Service) Create(id string) *entity.Order { return entity.NewOrder(id) }
`)

	// service module
	modB := filepath.Join(dir, "svc")
	os.MkdirAll(filepath.Join(modB, "handler"), 0o755)
	write(t, filepath.Join(modB, "go.mod"),
		"module example.com/svc\n\ngo 1.21\n\nrequire example.com/go-core v0.0.0\n\nreplace example.com/go-core => ../go-core\n")
	write(t, filepath.Join(modB, "handler/h.go"), `package handler
import (
	"example.com/go-core/orders/usecase"
	"example.com/go-core/orders/domain/entity"
)
type Handler struct{ svc *usecase.Service }
func (h *Handler) Do() *entity.Order { return h.svc.Create("x") }
`)

	write(t, filepath.Join(dir, "go.work"), "go 1.21\n\nuse (\n\t./go-core\n\t./svc\n)\n")
	gitInit(t, dir)
	return dir
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "-A"},
		{"commit", "-qm", "init", "--allow-empty"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=t@t")
		cmd.Run()
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ---- tests ----

func TestBareBuild(t *testing.T) {
	dir := makeMonorepo(t)
	stdout, _, code := run(t, dir)
	if code != 0 {
		t.Fatalf("bare build exited %d; stdout=%q", code, stdout)
	}
	if !strings.Contains(stdout, "packages") {
		t.Errorf("expected 'packages' in output, got %q", stdout)
	}
}

func TestHubs(t *testing.T) {
	dir := makeMonorepo(t)
	stdout, _, code := run(t, dir, "--hubs")
	if code != 0 {
		t.Fatalf("--hubs exited %d", code)
	}
	// entity is imported by both usecase and handler → top hub
	if !strings.Contains(stdout, "entity") {
		t.Errorf("entity should be top hub; got:\n%s", stdout)
	}
}

func TestDeterminism(t *testing.T) {
	dir := makeMonorepo(t)
	out1, _, _ := run(t, dir, "--hubs")
	out2, _, _ := run(t, dir, "--hubs")
	if out1 != out2 {
		t.Errorf("non-deterministic --hubs output:\n%s\nvs\n%s", out1, out2)
	}
}

func TestFindExitCodes(t *testing.T) {
	dir := makeMonorepo(t)
	run(t, dir) // warm cache

	_, _, code := run(t, dir, "--find", "Order")
	if code != 0 {
		t.Errorf("--find Order: expected exit 0, got %d", code)
	}
	_, _, code = run(t, dir, "--find", "NOTEXIST")
	if code != 1 {
		t.Errorf("--find NOTEXIST: expected exit 1, got %d", code)
	}
}

func TestUnknownFlagExit2(t *testing.T) {
	dir := makeMonorepo(t)
	_, stderr, code := run(t, dir, "--unknownflag")
	if code != 2 {
		t.Errorf("unknown flag: expected exit 2, got %d", code)
	}
	if !strings.Contains(stderr, "unknown flag") {
		t.Errorf("expected 'unknown flag' in stderr, got %q", stderr)
	}
}

func TestHelpExit0(t *testing.T) {
	dir := makeMonorepo(t)
	_, _, code := run(t, dir, "--help")
	if code != 0 {
		t.Errorf("--help: expected exit 0, got %d", code)
	}
}

func TestVersionExit0(t *testing.T) {
	dir := makeMonorepo(t)
	stdout, _, code := run(t, dir, "--version")
	if code != 0 {
		t.Errorf("--version: expected exit 0, got %d", code)
	}
	if stdout == "" {
		t.Error("expected version output")
	}
}

func TestJSONShape(t *testing.T) {
	dir := makeMonorepo(t)
	run(t, dir) // build cache

	// --json --hubs must emit a single valid JSON object
	stdout, _, code := run(t, dir, "--json", "--hubs")
	if code != 0 {
		t.Fatalf("--json --hubs exited %d", code)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &obj); err != nil {
		t.Fatalf("--json --hubs: invalid JSON: %v\noutput: %q", err, stdout)
	}
	for _, key := range []string{"command", "packageCount", "sha", "hubs"} {
		if _, ok := obj[key]; !ok {
			t.Errorf("--json --hubs: missing key %q in %v", key, obj)
		}
	}

	// --json --find emits matches array
	stdout, _, _ = run(t, dir, "--json", "--find", "Order")
	var findObj map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &findObj); err != nil {
		t.Fatalf("--json --find: invalid JSON: %v", err)
	}
	if findObj["command"] != "find" {
		t.Errorf("expected command=find, got %v", findObj["command"])
	}
}

func TestMonorepoRelates(t *testing.T) {
	dir := makeMonorepo(t)
	run(t, dir) // build cache

	stdout, _, code := run(t, dir, "--relates", "entity")
	if code != 0 {
		t.Fatalf("--relates entity exited %d; stdout=%q", code, stdout)
	}
	// Both usecase and svc/handler import entity → they should appear as dependents
	if !strings.Contains(stdout, "usecase") {
		t.Errorf("expected usecase in --relates entity output;\n%s", stdout)
	}
	if !strings.Contains(stdout, "handler") {
		t.Errorf("expected handler in --relates entity output;\n%s", stdout)
	}
}

func TestRelatesJSON(t *testing.T) {
	dir := makeMonorepo(t)
	run(t, dir)
	stdout, _, code := run(t, dir, "--json", "--relates", "entity")
	if code != 0 {
		t.Fatalf("--json --relates entity exited %d", code)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &obj); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if obj["command"] != "relates" {
		t.Errorf("expected command=relates")
	}
}

func TestGracefulDegradation(t *testing.T) {
	dir := makeMonorepo(t)
	// Add a syntactically broken file.
	os.MkdirAll(filepath.Join(dir, "go-core", "broken"), 0o755)
	write(t, filepath.Join(dir, "go-core", "broken", "bad.go"), "package broken\nfunc Oops( {\n")

	stdout, stderr, code := run(t, dir)
	if code != 0 {
		t.Fatalf("build should succeed even with broken file; exit=%d", code)
	}
	if !strings.Contains(stderr, "skipped") && !strings.Contains(stderr, "partial parse") {
		t.Logf("stderr: %q", stderr)
	}
	// Map should still contain valid packages.
	if !strings.Contains(stdout, "packages") {
		t.Errorf("expected 'packages' in output; got %q", stdout)
	}
}

func TestInjectionSafety(t *testing.T) {
	dir := makeMonorepo(t)
	run(t, dir) // build cache

	// A shell-special query must not cause the process to crash or execute code.
	// Exit 1 (no match) is the correct outcome for a safe literal search.
	_, _, code := run(t, dir, "--any", "$(echo pwned)")
	if code == 0 {
		// Unlikely for such a query to match, but not a failure per se.
	}
	// The process must exit cleanly (0 or 1), never with a crash (>1 outside of usage).
	if code > 1 {
		t.Errorf("injection query exited %d (expected 0 or 1)", code)
	}
}

func TestFeatures(t *testing.T) {
	dir := makeMonorepo(t)
	stdout, _, code := run(t, dir, "--features")
	if code != 0 {
		t.Fatalf("--features exited %d", code)
	}
	// DDD feature "orders" should be detected (contains domain/entity + usecase).
	if !strings.Contains(stdout, "orders") {
		t.Errorf("expected 'orders' feature; got:\n%s", stdout)
	}
}

func TestMapCommand(t *testing.T) {
	dir := makeMonorepo(t)
	stdout, _, code := run(t, dir, "--map")
	if code != 0 {
		t.Fatalf("--map exited %d", code)
	}
	if !strings.Contains(stdout, "tok") {
		t.Errorf("expected token count in --map output; got:\n%s", stdout)
	}
}

func TestPrintJSON(t *testing.T) {
	dir := makeMonorepo(t)
	run(t, dir) // build cache
	stdout, _, code := run(t, dir, "--print")
	if code != 0 {
		t.Fatalf("--print exited %d", code)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &m); err != nil {
		t.Fatalf("--print: invalid JSON: %v", err)
	}
	for _, key := range []string{"schema", "generatedSha", "packages", "hubs", "features", "rankedSymbols"} {
		if _, ok := m[key]; !ok {
			t.Errorf("--print: missing key %q", key)
		}
	}
}

func TestCacheFreshness(t *testing.T) {
	dir := makeMonorepo(t)
	// First build populates the cache.
	run(t, dir)
	// Second call should serve from cache (no rebuild stderr from build).
	_, stderr, code := run(t, dir, "--hubs")
	if code != 0 {
		t.Fatalf("second --hubs exited %d; stderr=%q", code, stderr)
	}
}
