package main

import (
	"context"

	"github.com/urfave/cli/v2"
)

var createOCIIndex = &cli.Command{
	Name:      "create-oci-index",
	Usage:     "create-oci-index",
	ArgsUsage: "<login-server>",
	Flags:     commonFlags,
	Action:    runGenerateOCIIndex,
}

func runGenerateOCIIndex(ctx *cli.Context) (err error) {
	proxy, err := proxy(ctx)
	if err != nil {
		return err
	}

	ctxu := context.Background()
	err = proxy.GenerateOCIIndex(ctxu, false)
	if err != nil {
		return err
	}

	return nil
}
