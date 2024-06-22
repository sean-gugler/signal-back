package cmd

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"syscall"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"github.com/xeals/signal-back/types"
	"golang.org/x/crypto/ssh/terminal"
)

// AppHelp is the help template.
const AppHelp = `About:
  {{.Name}}{{if .Usage}}: {{.Usage}}{{end}}{{if .Version}}{{if not .HideVersion}}
  Version {{.Version}}{{end}}{{end}}

Usage: {{if .UsageText}}{{.UsageText}}{{else}}{{.HelpName}} COMMAND [OPTION...] {{.ArgsUsage}}{{end}}

  {{range .VisibleFlags}}{{.}}
  {{end}}{{if .VisibleCommands}}
Commands:
{{range .VisibleCommands}}  {{index .Names 0}}{{ "\t"}}{{.Usage}}
{{end}}{{end}}
`

// TODO: Work out how to display global flags here
// SubcommandHelp is the subcommand help template.
const SubcommandHelp = `Usage: {{if .UsageText}}{{.UsageText}}{{else}}{{.HelpName}} [OPTION...] {{.ArgsUsage}}{{end}}{{if .Description}}

{{.Description}}{{end}}{{if .VisibleFlags}}

  {{range .VisibleFlags}}{{.}}
  {{end}}{{end}}
`

var coreFlags = []cli.Flag{
	&cli.StringFlag{
		Name:  "password, p",
		Usage: "use `PASS` as password for backup file",
	},
	&cli.StringFlag{
		Name:  "pwdfile, P",
		Usage: "read password from `FILE`",
	},
	&cli.BoolFlag{
		Name:  "verbose, v",
		Usage: "enable verbose logging output",
	},
}

func setup(c *cli.Context) (*types.BackupFile, error) {
	// -- Enable logging

	if c.Bool("verbose") {
		log.SetOutput(os.Stderr)
	} else {
		log.SetOutput(ioutil.Discard)
	}

	// -- Verify

	if c.Args().Get(0) == "" {
		return nil, errors.New("must specify a Signal backup file")
	}

	// -- Initialise

	pass, err := readPassword(c)
	if err != nil {
		return nil, errors.Wrap(err, "unable to read password")
	}

	bf, err := types.NewBackupFile(c.Args().Get(0), pass)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open backup file")
	}

	return bf, nil
}

func readPassword(c *cli.Context) (string, error) {
	var pass string

	if c.String("password") != "" {
		pass = c.String("password")
	} else if c.String("pwdfile") != "" {
		bs, err := ioutil.ReadFile(c.String("pwdfile"))
		if err != nil {
			return "", errors.Wrap(err, "unable to read file")
		}
		pass = string(bs)
	} else {
		// Read from stdin
		fmt.Fprint(os.Stderr, "Password: ")
		raw, err := terminal.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return "", errors.Wrap(err, "unable to read from stdin")
		}
		fmt.Fprint(os.Stderr, "\n")
		pass = string(raw)
	}
	return pass, nil
}
