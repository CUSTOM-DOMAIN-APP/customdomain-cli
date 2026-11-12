// browser.go opens the device approval page in the person's default browser. It runs the platform's own opener
// with the URL as one argument (no shell), and only for https links the API returned; failure is silent because
// the link is always printed too.
package cli

import (
	"os/exec"
	"runtime"
)

func openBrowser(u string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
