package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli"
	"github.com/xeals/signal-back/cmd"
	"github.com/xeals/signal-back/types"
	_ "modernc.org/sqlite"
)

var version = "devel"

func main() {
	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("signal-back %s\nproto commit: %s\n", version, types.ProtoCommitHash)
	}

	app := cli.NewApp()
	app.CustomAppHelpTemplate = cmd.AppHelp
	app.Usage = "extract and format the contents of Signal backup files"
	app.Name = appname
	app.Version = version
	app.Commands = []cli.Command{
		cmd.Analyse,
		cmd.Extract,
		cmd.Format,
	}
	app.ArgsUsage = "BACKUPFILE"
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "help, h",
			Usage: "show help",
		},
	}
	app.Action = func(c *cli.Context) error {
		return cli.ShowAppHelp(c)
	}
	// app.Action = cli.ActionFunc(func(c *cli.Context) error {
	// 	// -- Logging

	// 	if c.String("log") != "" {
	// 		f, err := os.OpenFile(c.String("log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	// 		if err != nil {
	// 			return errors.Wrap(err, "unable to create logging file")
	// 		}
	// 		logger = f
	// 	} else {
	// 		logger = os.Stderr
	// 	}
	// 	return nil
	// })

	if err := app.Run(os.Args); err != nil {
		// log.Fatalln(err)
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}
