// Package cli is the customer command line client: `customdomain login` signs in with device
// sign-in (RFC 8628) and keeps an agent grant's tokens on disk, and the other commands call the public API with them
// or with an API key from CUSTOMDOMAIN_API_KEY. It talks only to the public API and the documented OAuth endpoints and
// imports no API server code. Standard library only, so the binary is one static file.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

// DefaultAPI is the hosted API. CUSTOMDOMAIN_API_URL overrides it (a test mode or self-run API server).
const DefaultAPI = "https://api.customdomain.ai"

// DefaultScope is what `login` asks for when --scope is not given.
const DefaultScope = "domains:read domains:connect"

// clientName is how the CLI names itself to the authorization server; a person sees it on the consent page.
const clientName = "CustomDomain™ CLI"

// loopbackRedirect is the redirect the CLI registers. Device sign-in binds the code to it but never sends a browser
// there; registration requires one.
const loopbackRedirect = "http://127.0.0.1/callback"

// Credentials are what `login` keeps: the registered client and the grant's tokens.
type Credentials struct {
	API          string    `json:"api"`
	ClientID     string    `json:"client_id"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Scope        string    `json:"scope,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// App is one run of the CLI with its environment, injectable for tests.
type App struct {
	API      string
	APIKey   string
	CredPath string
	HTTP     *http.Client
	Out      io.Writer
	Err      io.Writer
	Now      func() time.Time
	Sleep    func(context.Context, time.Duration) error
	// OpenURL opens the approval page in the person's browser; nil (tests, CUSTOMDOMAIN_NO_BROWSER) only prints it.
	OpenURL func(string) error
}

// Version is the release version, set at build time with -ldflags "-X .../internal/cli.Version=1.2.3".
var Version = "dev"

// userAgent names the CLI and its version on every request, so sign-ins and calls from a terminal can be counted.
func userAgent() string { return "customdomain-cli/" + Version }

// FromEnv builds an App from the process environment.
func FromEnv() *App {
	api := strings.TrimRight(os.Getenv("CUSTOMDOMAIN_API_URL"), "/")
	if api == "" {
		api = DefaultAPI
	}
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, ".config")
		}
	}
	var opener func(string) error
	if os.Getenv("CUSTOMDOMAIN_NO_BROWSER") == "" {
		opener = openBrowser
	}
	return &App{
		OpenURL: opener,
		API:     api, APIKey: strings.TrimSpace(os.Getenv("CUSTOMDOMAIN_API_KEY")),
		CredPath: filepath.Join(dir, "customdomain", "credentials.json"),
		HTTP:     &http.Client{Timeout: 30 * time.Second}, Out: os.Stdout, Err: os.Stderr,
		Now: time.Now, Sleep: sleepCtx,
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Usage is the help text.
const Usage = `customdomain: the CustomDomain command line client

Usage:
  customdomain login [--scope "domains:read domains:connect"]   sign in from this terminal
  customdomain logout                                          end this terminal's access
  customdomain whoami                                          who this terminal acts as
  customdomain domains                                         list your connected domains
  customdomain doctor <domain>                                 check where a domain's DNS lives and what is in the way
  customdomain init [--app app_123] [--env-file .env.local]    create an API key and save it to the project's env file
  customdomain version                                         print the version

An API key in CUSTOMDOMAIN_API_KEY is used instead of a sign in when it is set.
Set CUSTOMDOMAIN_NO_BROWSER=1 to only print the approval link instead of opening it.
`

// Run executes one command line and returns the process exit code.
func (a *App) Run(ctx context.Context, args []string) int {
	if len(args) == 0 {
		fmt.Fprint(a.Err, Usage)
		return 2
	}
	var err error
	switch args[0] {
	case "login":
		err = a.login(ctx, args[1:])
	case "logout":
		err = a.logout(ctx)
	case "whoami":
		err = a.whoami(ctx)
	case "domains":
		err = a.domains(ctx)
	case "init":
		err = a.init(ctx, args[1:])
	case "doctor":
		if len(args) != 2 {
			fmt.Fprint(a.Err, Usage)
			return 2
		}
		err = a.doctor(ctx, args[1])
	case "version", "--version", "-v":
		fmt.Fprintln(a.Out, "customdomain", Version)
		return 0
	case "help", "-h", "--help":
		fmt.Fprint(a.Out, Usage)
		return 0
	default:
		fmt.Fprintf(a.Err, "unknown command %q\n\n%s", args[0], Usage)
		return 2
	}
	if err != nil {
		fmt.Fprintln(a.Err, "error:", err)
		return 1
	}
	return 0
}

// ---- HTTP ------------------------------------------------------------------

// apiError is the API server's error envelope, or an OAuth error.
type apiError struct {
	Status  int
	Code    string `json:"code"`
	Details string `json:"details"`
	Title   string `json:"title"`
	OAuth   string `json:"error"`
	Desc    string `json:"error_description"`
}

func (e *apiError) Error() string {
	switch {
	case e.Details != "":
		return e.Details
	case e.Desc != "":
		return e.Desc
	case e.Title != "":
		return e.Title
	case e.Code != "":
		return e.Code
	case e.OAuth != "":
		return e.OAuth
	}
	return fmt.Sprintf("HTTP %d", e.Status)
}

func (a *App) do(req *http.Request, out any) error {
	resp, err := a.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach %s: %w", a.API, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		e := &apiError{Status: resp.StatusCode}
		_ = json.Unmarshal(body, e)
		return e
	}
	if out != nil && len(body) > 0 {
		return json.Unmarshal(body, out)
	}
	return nil
}

func (a *App) postForm(ctx context.Context, path string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.API+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent())
	return a.do(req, out)
}

func (a *App) call(ctx context.Context, method, path string, in, out any) error {
	bearer, err := a.bearer(ctx)
	if err != nil {
		return err
	}
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.API+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("X-CustomDomain-Client", "cli")
	req.Header.Set("User-Agent", userAgent())
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return a.do(req, out)
}

// ---- credentials -----------------------------------------------------------

var errNotSignedIn = errors.New("not signed in: run `customdomain login`, or set CUSTOMDOMAIN_API_KEY")

func (a *App) load() (*Credentials, error) {
	b, err := os.ReadFile(a.CredPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, errNotSignedIn
	}
	if err != nil {
		return nil, err
	}
	var c Credentials
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("%s is not readable; run `customdomain login` again", a.CredPath)
	}
	return &c, nil
}

// save writes c readable by this user only.
func (a *App) save(c *Credentials) error {
	if err := os.MkdirAll(filepath.Dir(a.CredPath), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	tmp := a.CredPath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, a.CredPath)
}

// bearer is the credential for an API call: the API key when set, else the stored access token, refreshed first
// when it is about to expire.
func (a *App) bearer(ctx context.Context) (string, error) {
	if a.APIKey != "" {
		return a.APIKey, nil
	}
	c, err := a.load()
	if err != nil {
		return "", err
	}
	if a.Now().Before(c.ExpiresAt.Add(-30 * time.Second)) {
		return c.AccessToken, nil
	}
	var tok tokenResponse
	if err := a.postForm(ctx, "/oauth/agent/token", url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {c.RefreshToken}, "client_id": {c.ClientID},
	}, &tok); err != nil {
		return "", fmt.Errorf("this terminal's access ended (%v); run `customdomain login` again", err)
	}
	c.AccessToken, c.ExpiresAt = tok.AccessToken, a.Now().Add(time.Duration(tok.ExpiresIn)*time.Second)
	if tok.RefreshToken != "" {
		c.RefreshToken = tok.RefreshToken
	}
	if err := a.save(c); err != nil {
		return "", err
	}
	return c.AccessToken, nil
}

// ---- login / logout ----------------------------------------------------------

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

type deviceResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

func (a *App) login(ctx context.Context, args []string) error {
	scope := DefaultScope
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--scope" && i+1 < len(args):
			scope = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--scope="):
			scope = strings.TrimPrefix(args[i], "--scope=")
		default:
			return fmt.Errorf("unknown option %q", args[i])
		}
	}
	var reg struct {
		ClientID string `json:"client_id"`
	}
	b, _ := json.Marshal(map[string]any{"client_name": clientName, "redirect_uris": []string{loopbackRedirect}, "token_endpoint_auth_method": "none"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.API+"/oauth/agent/register", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent())
	if err := a.do(req, &reg); err != nil {
		return fmt.Errorf("could not register this terminal: %w", err)
	}
	var dev deviceResponse
	if err := a.postForm(ctx, "/oauth/agent/device_authorization", url.Values{"client_id": {reg.ClientID}, "scope": {scope}}, &dev); err != nil {
		var ae *apiError
		if errors.As(err, &ae) && ae.Status == http.StatusNotFound {
			return errors.New("signing in from a terminal is not available yet; set CUSTOMDOMAIN_API_KEY to an API key instead")
		}
		return err
	}
	open := dev.VerificationURIComplete
	if open == "" {
		open = dev.VerificationURI
	}
	fmt.Fprintf(a.Out, "Open %s\nand confirm the code %s. Waiting for approval...\n", open, dev.UserCode)
	if a.OpenURL != nil && strings.HasPrefix(open, "https://") {
		_ = a.OpenURL(open) // best effort: the link above is the fallback
	}
	interval := time.Duration(dev.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	deadline := a.Now().Add(time.Duration(dev.ExpiresIn) * time.Second)
	for {
		if err := a.Sleep(ctx, interval); err != nil {
			return err
		}
		var tok tokenResponse
		err := a.postForm(ctx, "/oauth/agent/token", url.Values{
			"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}, "device_code": {dev.DeviceCode}, "client_id": {reg.ClientID},
		}, &tok)
		if err == nil {
			c := &Credentials{API: a.API, ClientID: reg.ClientID, AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken,
				Scope: tok.Scope, ExpiresAt: a.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)}
			if err := a.save(c); err != nil {
				return err
			}
			fmt.Fprintln(a.Out, "Signed in. You can see and end this access under Agents in the console.")
			return nil
		}
		var ae *apiError
		if !errors.As(err, &ae) {
			return err
		}
		switch ae.OAuth {
		case "authorization_pending":
		case "slow_down":
			interval += 5 * time.Second
		case "expired_token":
			return errors.New("the code expired before it was approved; run `customdomain login` again")
		case "access_denied":
			return errors.New("the sign in was declined")
		default:
			return err
		}
		if !a.Now().Before(deadline) {
			return errors.New("the code expired before it was approved; run `customdomain login` again")
		}
	}
}

func (a *App) logout(ctx context.Context) error {
	c, err := a.load()
	if err != nil {
		return err
	}
	// Revoking the refresh token ends the grant; a failure here still forgets the tokens locally.
	_ = a.postForm(ctx, "/oauth/agent/revoke", url.Values{"token": {c.RefreshToken}, "client_id": {c.ClientID}}, nil)
	if err := os.Remove(a.CredPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	fmt.Fprintln(a.Out, "Signed out.")
	return nil
}

// ---- commands ----------------------------------------------------------------

func (a *App) whoami(ctx context.Context) error {
	if a.APIKey != "" {
		var me struct {
			TenantID   string `json:"tenant_id"`
			Role       string `json:"role"`
			Credential string `json:"credential"`
			Member     *struct {
				Email string `json:"email"`
			} `json:"member"`
		}
		if err := a.call(ctx, http.MethodGet, "/v1/me", nil, &me); err != nil {
			return err
		}
		who := "a workspace key"
		if me.Member != nil {
			who = me.Member.Email
		}
		fmt.Fprintf(a.Out, "API key (%s) in workspace %s, acting as %s with the %s role\n", me.Credential, me.TenantID, who, me.Role)
		return nil
	}
	c, err := a.load()
	if err != nil {
		return err
	}
	scope := c.Scope
	if scope == "" {
		scope = "(as approved)"
	}
	fmt.Fprintf(a.Out, "Signed in from this terminal (client %s) against %s, scopes: %s\n", c.ClientID, c.API, scope)
	return nil
}

type connection struct {
	Domain string `json:"domain"`
	Status string `json:"status"`
}

func (a *App) domains(ctx context.Context) error {
	var out struct {
		Connections []connection `json:"connections"`
	}
	if err := a.call(ctx, http.MethodGet, "/v1/connections", nil, &out); err != nil {
		return err
	}
	if len(out.Connections) == 0 {
		fmt.Fprintln(a.Out, "No domains yet.")
		return nil
	}
	sort.Slice(out.Connections, func(i, j int) bool { return out.Connections[i].Domain < out.Connections[j].Domain })
	tw := tabwriter.NewWriter(a.Out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "DOMAIN\tSTATUS")
	for _, c := range out.Connections {
		fmt.Fprintf(tw, "%s\t%s\n", c.Domain, c.Status)
	}
	return tw.Flush()
}

// checkFinding is one record_conflicts or advisories entry of POST /v1/domains:check.
type checkFinding struct {
	Kind     string `json:"kind"`
	Host     string `json:"host"`
	Type     string `json:"type"`
	Existing string `json:"existing"`
}

func (a *App) doctor(ctx context.Context, domain string) error {
	var out struct {
		Provider          *string        `json:"provider"`
		SetupType         string         `json:"setup_type"`
		SupportsAutomatic bool           `json:"supports_automatic"`
		ApexMessage       string         `json:"apex_message"`
		RecordConflicts   []checkFinding `json:"record_conflicts"`
		Advisories        []checkFinding `json:"advisories"`
	}
	if err := a.call(ctx, http.MethodPost, "/v1/domains:check", map[string]any{"domain": domain}, &out); err != nil {
		return err
	}
	provider := "unknown"
	if out.Provider != nil && *out.Provider != "" {
		provider = *out.Provider
	}
	auto := "by hand"
	if out.SupportsAutomatic {
		auto = "automatic"
	}
	fmt.Fprintf(a.Out, "%s\n  DNS is at:   %s\n  Setup:       %s (%s)\n", domain, provider, out.SetupType, auto)
	if out.ApexMessage != "" {
		fmt.Fprintf(a.Out, "  Apex:        %s\n", out.ApexMessage)
	}
	seen := map[string]bool{}
	for _, c := range out.Advisories {
		seen[c.Kind+c.Host+c.Type] = true
		fmt.Fprintf(a.Out, "  Note:        %s on %s (%s %s)\n", c.Kind, c.Host, c.Type, c.Existing)
	}
	conflicts := 0
	for _, c := range out.RecordConflicts {
		if seen[c.Kind+c.Host+c.Type] { // an advisory is also listed among the conflicts within /v1
			continue
		}
		conflicts++
		fmt.Fprintf(a.Out, "  In the way:  %s on %s (%s %s)\n", c.Kind, c.Host, c.Type, c.Existing)
	}
	if conflicts == 0 && len(out.Advisories) == 0 {
		fmt.Fprintln(a.Out, "  Nothing in the way.")
	}
	return nil
}
