package main

import (
	"os"

	"github.com/urfave/cli/v2"
)

// Set at linking time.
var (
	Version = "dev"
)

func main() {
	app := &cli.App{
		Name:    "generate image",
		Usage:   "",
		Version: Version,
		Authors: []*cli.Author{
			{
				Name: "Esteban Rey",
			},
		},
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  traceStr,
				Usage: "print trace logs with secrets",
			},
		},
		Commands: []*cli.Command{
			createOCIIndex,
		},
	}

	if err := app.Run(os.Args); err != nil {
		logger.Fatal().Msg(err.Error())
	}
}
