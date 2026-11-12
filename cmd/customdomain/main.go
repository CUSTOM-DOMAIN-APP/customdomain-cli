// Command customdomain is the CustomDomain™ command line client; the commands live in internal/cli.
// Release builds set the version with -ldflags "-X <module>/internal/cli.Version=<version>".
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/CUSTOM-DOMAIN-APP/customdomain-cli/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(cli.FromEnv().Run(ctx, os.Args[1:]))
}
