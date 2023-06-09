package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/estebanreyl/image-gen-test/pkg/registry"
	"github.com/rs/zerolog"
	"github.com/urfave/cli/v2"
)

// Common flag names
const (
	insecureStr     = "insecure"
	basicAuthStr    = "basicauth"
	userNameStr     = "username"
	passwordStr     = "password"
	dataEndpointStr = "dataendpoint"
	traceStr        = "trace"
)

// commonFlags is a collection of cli flags common to all commands.
var commonFlags = []cli.Flag{
	&cli.BoolFlag{
		Name:  insecureStr,
		Usage: "enable remote access over HTTP",
	},
	&cli.StringFlag{
		Name:    userNameStr,
		Aliases: []string{"u"},
		Usage:   "login username",
	},
	&cli.StringFlag{
		Name:    passwordStr,
		Aliases: []string{"p"},
		Usage:   "login password",
	},
	&cli.StringFlag{
		Name:    dataEndpointStr,
		Aliases: []string{"d"},
		Usage:   "endpoint for data download",
	},
	&cli.BoolFlag{
		Name:  basicAuthStr,
		Usage: "use basic auth mode for data operations",
	},
}

var (
	logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger()
)

// proxy creates an new proxy instance from context specific arguments and flags.
func proxy(ctx *cli.Context) (*registry.Proxy, error) {
	if ctx.Bool(traceStr) {
		logger = logger.With().Logger().Level(zerolog.TraceLevel)
	} else {
		logger = logger.With().Logger().Level(zerolog.InfoLevel)
	}

	username, password, basicAuthMode, err := getAuth(ctx)
	if err != nil {
		return nil, err
	}

	loginServer, dataEndpoint, err := resolveAll(ctx)
	if err != nil {
		return nil, err
	}

	return registry.NewProxy(
		&registry.Options{
			LoginServer:   loginServer,
			Username:      username,
			Password:      password,
			DataEndpoint:  dataEndpoint,
			Insecure:      ctx.Bool(insecureStr),
			BasicAuthMode: basicAuthMode,
		},
		logger)
}

// getAuth gets authentication information from context.
func getAuth(ctx *cli.Context) (username, password string, basicAuthMode bool, err error) {
	username = ctx.String(userNameStr)
	password = ctx.String(passwordStr)
	basicAuthMode = ctx.Bool(basicAuthStr)

	if username != "" && password == "" {
		err = errors.New("password required with username")
		return
	}

	if password != "" && username == "" {
		err = errors.New("username not specified")
		return
	}

	if username == "" && basicAuthMode {
		err = errors.New("cannot use basic auth without username")
		return
	}

	return username, password, basicAuthMode, nil
}

// resolveAll attempts to resolve the endpoints specified in the context.
func resolveAll(ctx *cli.Context) (loginServer, dataEndpoint string, err error) {
	hostnames := []string{}

	if loginServer = ctx.Args().First(); loginServer == "" {
		return loginServer, dataEndpoint, errors.New("login server name required")
	}

	hostnames = append(hostnames, loginServer)

	if dataEndpoint = ctx.String(dataEndpointStr); dataEndpoint != "" {
		hostnames = append(hostnames, dataEndpoint)
	}

	for _, hostname := range hostnames {
		if err := resolve(hostname); err != nil {
			return loginServer, dataEndpoint, err
		}
	}

	return loginServer, dataEndpoint, nil
}

// resolve ..
// dig +short hostname
func resolve(hostname string) error {
	if hostname == "" {
		return errors.New("hostname required")
	}

	path := []string{}

	cur := hostname
	path = append(path, cur)
	for {
		cname, err := net.LookupCNAME(cur)
		if err != nil {
			return err
		}
		if cname == cur {
			// No more aliases.
			break
		}
		path = append(path, cname)
		cur = cname
	}

	ip, err := net.LookupIP(cur)
	if err != nil {
		return err
	}
	path = append(path, ip[0].String())

	logger.Info().Msg(fmt.Sprintf("DNS:  %v", strings.Join(path, " -> ")))
	return nil
}
