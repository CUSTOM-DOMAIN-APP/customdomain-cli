// Tests for `customdomain init`: it signs in with the narrow scope, writes the key to the env file without printing
// it, keeps the file out of git, and never overwrites a key that is already set.
package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func initRig(t *testing.T, apps ...map[string]any) (*rig, string) {
	t.Helper()
	r := newRig(t)
	r.api.wantScope = InitScope
	r.api.apps = apps
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return r, dir
}

var (
	testApp = map[string]any{"id": "app_test", "name": "Acme test", "sandbox": true}
	liveApp = map[string]any{"id": "app_live", "name": "Acme", "sandbox": false}
)

func TestInitWritesTheKeyToTheEnvFileAndNeverPrintsIt(t *testing.T) {
	r, dir := initRig(t, liveApp, testApp)
	env := filepath.Join(dir, ".env.local")
	if code := r.app.Run(context.Background(), []string{"init", "--env-file", env}); code != 0 {
		t.Fatalf("init = %d: %s", code, r.out)
	}
	if strings.Contains(r.out.String(), "SECRETVALUE") {
		t.Fatalf("init printed the secret: %s", r.out)
	}
	if !strings.Contains(r.out.String(), "BCDF-GHJK") {
		t.Fatalf("init did not run the sign in: %s", r.out)
	}
	b, err := os.ReadFile(env)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b); got != "CUSTOMDOMAIN_API_KEY=cd_test_SECRETVALUE\nCUSTOMDOMAIN_APPLICATION_ID=app_test\n" {
		t.Fatalf("env file = %q", got)
	}
	if fi, _ := os.Stat(env); fi.Mode().Perm() != 0o600 {
		t.Fatalf("env file mode = %v, want 0600", fi.Mode().Perm())
	}
	gi, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if string(gi) != ".env.local\n" {
		t.Fatalf(".gitignore = %q", gi)
	}
}

func TestInitLeavesAKeyThatIsAlreadySet(t *testing.T) {
	r, dir := initRig(t, testApp)
	env := filepath.Join(dir, ".env.local")
	_ = os.WriteFile(env, []byte("CUSTOMDOMAIN_API_KEY=cd_live_old\n"), 0o600)
	if code := r.app.Run(context.Background(), []string{"init", "--env-file", env}); code != 0 {
		t.Fatalf("init = %d: %s", code, r.out)
	}
	if len(r.api.keyBodies) != 0 || r.api.polls != 0 {
		t.Fatalf("init signed in or created a key when one was set")
	}
}

func TestInitAsksWhichApplicationWhenItCannotTell(t *testing.T) {
	r, dir := initRig(t, liveApp, map[string]any{"id": "app_other", "name": "Other", "sandbox": false})
	code := r.app.Run(context.Background(), []string{"init", "--env-file", filepath.Join(dir, ".env")})
	if code == 0 || !strings.Contains(r.out.String(), "--app") || !strings.Contains(r.out.String(), "app_other") {
		t.Fatalf("init = %d: %s", code, r.out)
	}
}

func TestInitKeepsAGitignoreThatAlreadyCoversTheFile(t *testing.T) {
	r, dir := initRig(t, testApp)
	_ = os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules\n.env*.local\n"), 0o644)
	if code := r.app.Run(context.Background(), []string{"init", "--env-file=" + filepath.Join(dir, ".env.local")}); code != 0 {
		t.Fatalf("init = %d: %s", code, r.out)
	}
	if gi, _ := os.ReadFile(filepath.Join(dir, ".gitignore")); string(gi) != "node_modules\n.env*.local\n" {
		t.Fatalf(".gitignore changed: %q", gi)
	}
}

func TestVersionCommand(t *testing.T) {
	var out bytes.Buffer
	a := &App{Out: &out, Err: &out}
	if code := a.Run(context.Background(), []string{"version"}); code != 0 || !strings.Contains(out.String(), "customdomain "+Version) {
		t.Fatalf("code %d, out %q", code, out.String())
	}
}
