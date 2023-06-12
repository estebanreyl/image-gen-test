package main

import (
	"context"

	"github.com/urfave/cli/v2"
)

var createOCIArtifactsTest = &cli.Command{
	Name:      "create-oci-artifacts-test",
	Usage:     "create-oci-artifacts-test",
	ArgsUsage: "<login-server>",
	Flags:     commonFlags,
	Action:    runGenerateOCIArtifacts,
}

func runGenerateOCIArtifacts(ctx *cli.Context) (err error) {
	proxy, err := proxy(ctx)
	if err != nil {
		return err
	}

	ctxu := context.Background()
	err = proxy.GenerateOCIArtifacts(ctxu)
	if err != nil {
		return err
	}

	return nil
}
