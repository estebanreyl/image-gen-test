package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	rhttp "github.com/estebanreyl/image-gen-test/pkg/http"
	"github.com/estebanreyl/image-gen-test/pkg/io"
	"github.com/rs/zerolog"
)

// authType is an authorization type to use for an HTTP request.
type authType int

// The different kinds of auth mechanisms supported by transport.
const (
	noAuth authType = iota
	basicAuth
	bearerAuth
)

const (
	schemeBearer = "bearer"

	claimRealm   = "realm"
	claimService = "service"
	claimScope   = "scope"
)

var authHeaderRegex = regexp.MustCompile(`(realm|service|scope)="([^"]*)`)

// registryRequest describes content of a registry request.
type registryRequest struct {
	method      string
	url         string
	body        io.Reader
	contentType string
	accept      string
}

// transport can be used to make HTTP requests with authentication.
// Basic and bearer auth are supported.
type transport struct {
	tripper rhttp.RoundTripper
	authType
	username string
	password string
	logger   zerolog.Logger
}

// newTransport returns a new transport.
func newTransport(tripper rhttp.RoundTripper, username, password string, at authType, logger zerolog.Logger) (transport, error) {
	t := transport{}

	switch at {
	case bearerAuth, basicAuth:
		if username == "" {
			return t, errors.New("username required")
		}
		if password == "" {
			return t, errors.New("password required")
		}
	}

	if tripper == nil {
		return t, errors.New("round trippper required")
	}

	t.username = username
	t.password = password
	t.authType = at
	t.logger = logger
	t.tripper = tripper

	return t, nil
}

// newNoAuthTransport returns a new transport that does not use auth.
func newNoAuthTransport(tripper rhttp.RoundTripper, logger zerolog.Logger) (transport, error) {
	return newTransport(tripper, "", "", noAuth, logger)
}

// newBasicAuthTransport returns a new transport that uses basic auth.
func newBasicAuthTransport(tripper rhttp.RoundTripper, username, password string, logger zerolog.Logger) (transport, error) {
	return newTransport(tripper, username, password, basicAuth, logger)
}

// newBearerAuthTransport returns a new transport that uses bearer auth.
func newBearerAuthTransport(tripper rhttp.RoundTripper, username, password string, logger zerolog.Logger) (transport, error) {
	return newTransport(tripper, username, password, bearerAuth, logger)
}

// roundTrip makes an HTTP request and returns the response body.
// It supports basic and bearer authorization.
func (t transport) roundTrip(regReq registryRequest) (tripInfo rhttp.RoundTripInfo, err error) {
	req, err := http.NewRequest(regReq.method, regReq.url, regReq.body)
	if err != nil {
		return tripInfo, err
	}
	if regReq.contentType != "" {
		req.Header.Set(rhttp.HeaderContentType, regReq.contentType)
	}
	if regReq.accept != "" {
		req.Header.Set(rhttp.HeaderAccept, regReq.accept)
	}

	switch t.authType {
	case bearerAuth:
		tokenReq, err := http.NewRequest(regReq.method, regReq.url, nil)
		if err != nil {
			return tripInfo, err
		}
		tripInfo, err = t.tripper.RoundTrip(tokenReq)
		if err != nil {
			return tripInfo, err
		}
		if tripInfo.Response.Code != http.StatusUnauthorized {
			return tripInfo, errors.New("failed to get challenge")
		}
		scheme, params := parseAuthHeader(tripInfo.Response.HeaderChallenge)
		if scheme == schemeBearer {
			token, err := t.getToken(params)
			if err != nil {
				return tripInfo, err
			}

			req.Header.Set(rhttp.HeaderAuthorization, "Bearer "+token)
		} else {
			return tripInfo, errors.New("server does not support bearer authentication")
		}
	case basicAuth:
		if t.username == "" {
			return tripInfo, errors.New("username not provided")
		}
		req.SetBasicAuth(t.username, t.password)
	}

	tripInfo, err = t.tripper.RoundTrip(req)
	if err != nil {
		return tripInfo, err
	}

	return tripInfo, nil
}

// getToken attempts to get an auth token based on the given params.
// The params specify:
// - realm: the HTTP endpoint of the token server
// - service: the service to obtain the token for, such as myregistry.azurecr.io
// - scope: the authorization scope the token grants
func (t transport) getToken(params map[string]string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, params[claimRealm], nil)
	if err != nil {
		return "", err
	}
	if t.username != "" {
		req.SetBasicAuth(t.username, t.password)
	}

	query := url.Values{}
	if service, ok := params[claimService]; ok {
		query.Set(claimService, service)
	}
	if scope, ok := params[claimScope]; ok {
		query.Set(claimScope, scope)
	}
	req.URL.RawQuery = query.Encode()

	tripInfo, err := t.tripper.RoundTrip(req)
	if err != nil {
		return "", err
	}
	if tripInfo.Response.Code != http.StatusOK {
		return "", fmt.Errorf("get access token failed, expected: 200, got: %v", tripInfo.Response.Code)
	}

	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(tripInfo.Response.Body, &result); err != nil {
		return "", err
	}
	return result.AccessToken, nil
}

// parseAuthHeader parses the Www-Authenticate header and retrieves auth metadata
// that can be used to obtain auth tokens.
func parseAuthHeader(header string) (string, map[string]string) {
	parts := strings.SplitN(header, " ", 2)
	scheme := strings.ToLower(parts[0])
	if len(parts) < 2 {
		return scheme, nil
	}

	params := make(map[string]string)
	result := authHeaderRegex.FindAllStringSubmatch(parts[1], -1)
	for _, match := range result {
		params[strings.ToLower(match[1])] = match[2]
	}

	return scheme, params
}
