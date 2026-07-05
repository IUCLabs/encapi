// Command encapi-cli manages the Puppet ENC via the encapi HTTP API. Puppet's
// exec node_terminus calls `encapi-cli classify <certname>`.
package main

import (
	"os"

	"github.com/IUCLabs/encapi/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], cli.LoadEnv(), os.Stdout, os.Stderr))
}
