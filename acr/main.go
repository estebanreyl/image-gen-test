package main

import (
	"context"
	"io"
	"io/ioutil"
	"os"

	"github.com/sirupsen/logrus"

	containerdLog "github.com/containerd/containerd/log"
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
			createOCIArtifactsTest,
		},
	}
	disableLibraryLogrusLogging()

	if err := app.Run(os.Args); err != nil {
		logger.Fatal().Msg(err.Error())
	}
}

func disableLibraryLogrusLogging() {
	containerdLog.G = func(ctx context.Context) *logrus.Entry {
		logger := logrus.New()
		logger.SetOutput(io.Discard)
		return logrus.NewEntry(logger)
	}
	// disable all logrus logging from standard logrus logger
	logrus.SetOutput(ioutil.Discard)
}
