package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ociimagespec "github.com/opencontainers/image-spec/specs-go/v1"
	orasartifact "github.com/oras-project/artifacts-spec/specs-go/v1"
	"github.com/rs/zerolog"
	"github.com/sirupsen/logrus"
)

// Registry REST routes
const (
	// Ping routes
	routeFrontendPing     = "/v2/"
	routeDataEndpointPing = "/"

	// Blob routes
	routeInitiateBlobUpload = "/v2/%s/blobs/uploads/" // add repo name
	routeBlobPull           = "/v2/%s/blobs/%s"       // add repo name and digest

	// Manifest routes
	routeManifest = "/v2/%s/manifests/%s" // add repo name and digest/tag

	// Referrer routes
	// ocirouteReferrers = "/oras/artifacts/v1/%s/manifests/%s/referrers" // add repo name and digest
	ocirouteReferrers  = "/v2/%s/referrers/%s"                          // add repo name and digest
	orasrouteReferrers = "/oras/artifacts/v1/%s/manifests/%s/referrers" // add repo name and digest

	// API Versions
	OciReferrers         = "Referrers_OCI_V1"
	OciManifestReferrers = "Referrers_OCI_Manifest"
	OrasReferrers        = "Referrers_ORAS_V1"
)

// Constants for generated data.
const (
	checkHealthAuthor       = "ACR Check Health"
	checkHealthMediaType    = "application/acr.checkhealth.test"
	checkHealthArtifactType = "application/acr.checkhealth.artifact.test"
	checkHealthLayerFmt     = "Test layer authored by " + checkHealthAuthor + " at %s" // add time
	checkHealthRepoPrefix   = "acrcheckhealth"
)

// Other data.
var (
	ociConfig = ociimagespec.Image{
		Author: checkHealthAuthor,
	}
)

// referrersResponse describes the referrers API response.
// See: https://gist.github.com/aviral26/ca4b0c1989fd978e74be75cbf3f3ea92
type referrersResponse struct {
	// Referrers is a collection of referrers.
	Referrers []orasartifact.Descriptor `json:"references"`
}

// Options configures the proxy.
type Options struct {
	// LoginServer is the registry login server name, such as myregistry.azurecr.io
	LoginServer string

	// DataEndpoint is the registry data endpoint, such as myregistry.southindia.azurecr.io
	DataEndpoint string

	// Username is the registry login username
	Username string

	// Password is the registry login password
	Password string

	// Insecure indicates if registry should be accessed over HTTP
	Insecure bool

	// BasicAuthMode indicates that only basic auth should be used
	BasicAuthMode bool

	Repository string
}

// Proxy acts as a proxy to a remote registry.
type Proxy struct {
	*Options
	zerolog.Logger
	resolver remotes.Resolver
}

// NewProxy creates a new registry proxy.
func NewProxy(opts *Options, logger zerolog.Logger) (*Proxy, error) {
	if opts == nil {
		return nil, errors.New("opts required")
	}

	if opts.LoginServer == "" {
		return nil, errors.New("login server name required")
	}

	resolver := docker.NewResolver(docker.ResolverOptions{
		Credentials: func(s string) (string, string, error) {
			return opts.Username, opts.Password, nil
		},
		PlainHTTP: false,
	})

	return &Proxy{
		resolver: resolver,
		Options:  opts,
		Logger:   logger,
	}, nil
}

// PushOCIIndex pushes an OCI Index to the registry
func (p Proxy) GenerateOCIIndex(ctx context.Context, hasMediaType bool) error {
	var (
		repo = fmt.Sprintf("%v%v", checkHealthRepoPrefix, time.Now().Unix())
		tag  = fmt.Sprintf("%v", time.Now().Unix())
	)
	if p.Repository != "" {
		repo = p.Repository
	}

	var Manifests []ociimagespec.Descriptor
	for i := 0; i < 10; i++ {
		// Push simple image
		desc, err := p.pushOCIImage(ctx, repo, fmt.Sprintf("%s-oci-%d", tag, i), ociConfig, 2)
		if err != nil {
			return err
		}
		Manifests = append(Manifests, desc)
	}
	index := ociimagespec.Index{
		Versioned: specs.Versioned{
			SchemaVersion: 2,
		},
		Manifests: Manifests,
	}
	if hasMediaType {
		index.MediaType = ociimagespec.MediaTypeImageIndex
	}

	indexBytes, err := json.Marshal(index)
	if err != nil {
		return err
	}

	ref := fmt.Sprintf("%s/%s:%s", p.Options.LoginServer, repo, tag)
	pusher, err := p.resolver.Pusher(ctx, ref)
	if err != nil {
		return err
	}
	indexDesc := ociimagespec.Descriptor{
		MediaType: ociimagespec.MediaTypeImageIndex,
		Digest:    digest.FromBytes(indexBytes),
		Size:      int64(len(indexBytes)),
	}
	err = uploadBytes(ctx, pusher, indexDesc, indexBytes)
	if err != nil {
		return err
	}

	return nil
}

func (p Proxy) url(hostname, route string) string {
	scheme := "https"
	if p.Insecure {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s%s", scheme, hostname, route)
}

func (p Proxy) pushOCIImage(ctx context.Context, repo, tag string, config any, layercount int) (ociimagespec.Descriptor, error) {
	configBytes, err := json.Marshal(config)
	if err != nil {
		return ociimagespec.Descriptor{}, err
	}

	ref := fmt.Sprintf("%s/%s:%s", p.Options.LoginServer, repo, tag)
	pusher, err := p.resolver.Pusher(ctx, ref)
	// Upload config blob
	if err != nil {
		return ociimagespec.Descriptor{}, err
	}
	configDesc := ociimagespec.Descriptor{
		MediaType: checkHealthMediaType,
		Digest:    digest.FromBytes(configBytes),
		Size:      int64(len(configBytes)),
	}
	err = uploadBytes(ctx, pusher, configDesc, configBytes)
	if err != nil {
		return ociimagespec.Descriptor{}, err
	}

	// upload layers
	var layerDigests []ociimagespec.Descriptor
	for i := 0; i < layercount; i++ {
		layerBytes := []byte(fmt.Sprintf(checkHealthLayerFmt, time.Now(), i))
		layerDesc := ociimagespec.Descriptor{
			MediaType: ociimagespec.MediaTypeImageLayer,
			Digest:    digest.FromBytes(configBytes),
			Size:      int64(len(configBytes)),
		}
		err := uploadBytes(ctx, pusher, layerDesc, layerBytes)
		if err != nil {
			return ociimagespec.Descriptor{}, err
		}
		layerDigests = append(layerDigests, layerDesc)
	}

	ociManifest := ociimagespec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ociimagespec.MediaTypeImageManifest,
		Config:    configDesc,
		Layers:    layerDigests,
	}

	manifestBytes, err := json.Marshal(ociManifest)
	if err != nil {
		return ociimagespec.Descriptor{}, err
	}

	// Upload manifest
	manifestDesc := ociimagespec.Descriptor{
		MediaType: ociimagespec.MediaTypeImageManifest,
		Digest:    digest.FromBytes(manifestBytes),
		Size:      int64(len(manifestBytes)),
	}
	err = uploadBytes(ctx, pusher, manifestDesc, manifestBytes)
	if err != nil {
		return ociimagespec.Descriptor{}, err
	}
	return manifestDesc, nil
}

func uploadBytes(ctx context.Context, pusher remotes.Pusher, desc ociimagespec.Descriptor, data []byte) error {
	cw, err := pusher.Push(ctx, desc)
	if err != nil {
		if errdefs.IsAlreadyExists(err) {
			logrus.Infof("content %s exists", desc.Digest.String())
			return nil
		}
		return err
	}
	defer cw.Close()

	err = content.Copy(ctx, cw, bytes.NewReader(data), desc.Size, desc.Digest)
	if err != nil {
		return err
	}
	return nil
}
