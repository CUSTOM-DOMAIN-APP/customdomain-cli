// `customdomain init`: sets up a project to embed CustomDomain™ without the API key ever passing through a chat or a
// terminal's scrollback. It signs in with device sign-in when needed, picks the application, creates a key and writes
// it to an env file (mode 0600, kept out of git). The secret is never printed; only its prefix is. A coding agent can
// run it for a person, who only approves the sign-in in the browser.
package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// InitScope is what `init` asks for: read the applications and create one key. It cannot change DNS, billing or members.
const InitScope = "app:read keys:manage"

// envKey and envApp are the variables `init` writes, the names the embed guide's server route reads.
const (
	envKey = "CUSTOMDOMAIN_API_KEY"
	envApp = "CUSTOMDOMAIN_APPLICATION_ID"
)

type initApp struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Environment string `json:"environment"`
	Sandbox     bool   `json:"sandbox"`
}

func (a *App) init(ctx context.Context, args []string) error {
	envFile, appID, keyEnv := ".env.local", "", ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		val := func(name string) (string, error) {
			if strings.HasPrefix(arg, name+"=") {
				return strings.TrimPrefix(arg, name+"="), nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s needs a value", name)
			}
			i++
			return args[i], nil
		}
		var err error
		switch {
		case arg == "--env-file" || strings.HasPrefix(arg, "--env-file="):
			envFile, err = val("--env-file")
		case arg == "--app" || strings.HasPrefix(arg, "--app="):
			appID, err = val("--app")
		case arg == "--environment" || strings.HasPrefix(arg, "--environment="):
			keyEnv, err = val("--environment")
		default:
			return fmt.Errorf("unknown option %q", arg)
		}
		if err != nil {
			return err
		}
	}

	existing, err := readEnvFile(envFile)
	if err != nil {
		return err
	}
	if existing[envKey] != "" {
		fmt.Fprintf(a.Out, "%s is already set in %s; nothing was changed. To replace it, revoke the old key in the console and remove the line.\n", envKey, envFile)
		return nil
	}

	if a.APIKey == "" {
		if err := a.ensureScope(ctx, InitScope); err != nil {
			return err
		}
	}

	var list struct {
		Applications []initApp `json:"applications"`
	}
	if err := a.call(ctx, "GET", "/v1/applications", nil, &list); err != nil {
		return fmt.Errorf("could not list your applications: %w", err)
	}
	app, err := pickApp(list.Applications, appID)
	if err != nil {
		return err
	}

	body := map[string]any{}
	if keyEnv != "" {
		body["environment"] = keyEnv
	}
	var key struct {
		ID     string `json:"id"`
		Prefix string `json:"prefix"`
		Key    string `json:"key"`
	}
	if err := a.call(ctx, "POST", "/v1/applications/"+url.PathEscape(app.ID)+"/keys", body, &key); err != nil {
		return fmt.Errorf("could not create a key: %w", err)
	}
	if key.Key == "" {
		return errors.New("the key was created but its secret was not returned; revoke it in the console and run init again")
	}

	lines := []string{envKey + "=" + key.Key}
	if existing[envApp] == "" {
		lines = append(lines, envApp+"="+app.ID)
	}
	if err := appendEnv(envFile, lines); err != nil {
		return fmt.Errorf("the key %s was created but could not be saved (%v); revoke it in the console", key.ID, err)
	}
	ignored, err := ignoreInGit(envFile)
	if err != nil {
		fmt.Fprintf(a.Err, "warning: could not add %s to .gitignore: %v\n", filepath.Base(envFile), err)
	}

	label := app.Name
	if app.Sandbox {
		label += ", test mode"
	}
	fmt.Fprintf(a.Out, "Application: %s (%s)\n", app.ID, label)
	fmt.Fprintf(a.Out, "Created the API key %s (%s...) and wrote %s to %s.\n", key.ID, key.Prefix, envKey, envFile)
	if ignored {
		fmt.Fprintf(a.Out, "Added %s to .gitignore.\n", filepath.Base(envFile))
	}
	fmt.Fprintln(a.Out, "Set the same variable in your host's settings when you deploy. The key is not shown here; the console can revoke it.")
	return nil
}

// ensureScope signs in when there is no sign-in, or when the stored one lacks a scope `need` asks for.
func (a *App) ensureScope(ctx context.Context, need string) error {
	c, err := a.load()
	if err == nil && hasScopes(c.Scope, need) {
		return nil
	}
	if err != nil && !errors.Is(err, errNotSignedIn) {
		return err
	}
	scope := need
	if c != nil && c.Scope != "" {
		scope = mergeScopes(c.Scope, need)
	}
	fmt.Fprintln(a.Out, "Sign in to let this project create its API key.")
	return a.login(ctx, []string{"--scope", scope})
}

func hasScopes(have, need string) bool {
	set := map[string]bool{}
	for _, s := range strings.Fields(have) {
		set[s] = true
	}
	for _, s := range strings.Fields(need) {
		if !set[s] {
			return false
		}
	}
	return true
}

func mergeScopes(a, b string) string {
	seen, out := map[string]bool{}, []string{}
	for _, s := range strings.Fields(a + " " + b) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return strings.Join(out, " ")
}

// pickApp is the application `--app` names, or the only one, or the only test mode one.
func pickApp(apps []initApp, id string) (initApp, error) {
	if id != "" {
		for _, a := range apps {
			if a.ID == id {
				return a, nil
			}
		}
		return initApp{}, fmt.Errorf("no application %s in this workspace", id)
	}
	switch len(apps) {
	case 0:
		return initApp{}, errors.New("this workspace has no application yet; create one in the console, then run init again")
	case 1:
		return apps[0], nil
	}
	var sandboxes []initApp
	for _, a := range apps {
		if a.Sandbox {
			sandboxes = append(sandboxes, a)
		}
	}
	if len(sandboxes) == 1 {
		return sandboxes[0], nil
	}
	var b strings.Builder
	b.WriteString("this workspace has several applications; pick one with --app:")
	for _, a := range apps {
		mode := ""
		if a.Sandbox {
			mode = ", test mode"
		}
		fmt.Fprintf(&b, "\n  %s  %s%s", a.ID, a.Name, mode)
	}
	return initApp{}, errors.New(b.String())
}

// readEnvFile reads KEY=value lines; a missing file is empty.
func readEnvFile(path string) (map[string]string, error) {
	out := map[string]string{}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.HasPrefix(line, "#") {
			continue
		}
		out[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	return out, sc.Err()
}

// appendEnv adds lines to the env file, creating it readable by this user only.
func appendEnv(path string, lines []string) error {
	prefix := ""
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 && b[len(b)-1] != '\n' {
		prefix = "\n"
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(prefix + strings.Join(lines, "\n") + "\n"); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// ignoreInGit adds the env file's name to the .gitignore beside it when that directory is in a git checkout and the
// name is not already listed. It reports whether it added the line.
func ignoreInGit(envFile string) (bool, error) {
	abs, err := filepath.Abs(envFile)
	if err != nil {
		return false, err
	}
	dir, name := filepath.Dir(abs), filepath.Base(abs)
	if !inGitCheckout(dir) {
		return false, nil
	}
	gi := filepath.Join(dir, ".gitignore")
	b, err := os.ReadFile(gi)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l == name || l == "/"+name || l == ".env*" || l == ".env*.local" || l == "*.local" {
			return false, nil
		}
	}
	prefix := ""
	if len(b) > 0 && b[len(b)-1] != '\n' {
		prefix = "\n"
	}
	f, err := os.OpenFile(gi, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	if _, err := f.WriteString(prefix + name + "\n"); err != nil {
		f.Close()
		return false, err
	}
	return true, f.Close()
}

func inGitCheckout(dir string) bool {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}
