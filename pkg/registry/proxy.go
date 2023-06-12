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
	"github.com/google/uuid"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ociimagespec "github.com/opencontainers/image-spec/specs-go/v1"
	orasartifact "github.com/oras-project/artifacts-spec/specs-go/v1"
	"github.com/rs/zerolog"
	"github.com/sirupsen/logrus"
)

// Registry REST routes
const (
	// Referrer routes
	ocirouteReferrers = "/v2/%s/referrers/%s" // add repo name and digest
)

// Constants for generated data.
const (
	author                  = "esrey"
	imagegenConfigMediaType = "application/acr.imagegent.test"
	imagegenArtifactType    = "application/acr.imagegent.artifact.test"
	repoprefix              = "imagegentest"
	tagPrefix               = "genimage"
)

// Other data.
var (
	ociConfig = ociimagespec.Image{
		Author: author,
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
		repo = fmt.Sprintf("%v%v", repoprefix, time.Now().Unix())
		tag  = fmt.Sprintf("%v", time.Now().Unix())
	)
	if p.Repository != "" {
		repo = p.Repository
	}

	var Manifests []ociimagespec.Descriptor
	for i := 0; i < 11; i++ {
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

type artifactConstructOptions struct {
	includesArtifactType bool
	configIsScratch      bool
	layersAreScratch     bool
	layercount           int
	hasSubject           bool
	subjectInRegistry    bool
	errorExpected        bool
}

func (p Proxy) GenerateOCIArtifacts(ctx context.Context) error {
	var (
		repo = fmt.Sprintf("%v%v", repoprefix, time.Now().Unix())
	)
	if p.Repository != "" {
		repo = p.Repository
	}
	// Subject Exists
	opts := []artifactConstructOptions{
		{
			// Basic Referrer Artifact type (Success)
			includesArtifactType: true,
			configIsScratch:      true,
			layersAreScratch:     true,
			layercount:           1,
			hasSubject:           true,
			subjectInRegistry:    true,
			errorExpected:        false,
		},
		{
			// Basic Refferer Artifact type (Success - teleportLike)
			includesArtifactType: true,
			configIsScratch:      false,
			layersAreScratch:     false,
			layercount:           3,
			hasSubject:           true,
			subjectInRegistry:    true,
			errorExpected:        false,
		},
		{
			// Artifact Type no artifact type but using scratch (Error)
			includesArtifactType: false,
			configIsScratch:      true,
			layersAreScratch:     true,
			layercount:           1,
			hasSubject:           true,
			subjectInRegistry:    true,
			errorExpected:        true,
		},
		{
			// Basic Artifact type with Layers and scratch config (Expected)
			includesArtifactType: true,
			configIsScratch:      true,
			layersAreScratch:     false,
			layercount:           1,
			hasSubject:           true,
			subjectInRegistry:    true,
			errorExpected:        false,
		},
		{
			// Basic Artifact type with config, scratch layers and no artifact type (Expected)
			includesArtifactType: false,
			configIsScratch:      false,
			layersAreScratch:     true,
			layercount:           1,
			hasSubject:           true,
			subjectInRegistry:    true,
			errorExpected:        false,
		},
		{
			// Basic Artifact type Scratch everything, no subject (Expected)
			includesArtifactType: true,
			configIsScratch:      true,
			layersAreScratch:     true,
			layercount:           1,
			hasSubject:           false,
			subjectInRegistry:    false,
			errorExpected:        false,
		},
		{
			// Basic Artifact type Scratch everything, no artifact type (Expected)
			includesArtifactType: false,
			configIsScratch:      true,
			layersAreScratch:     true,
			layercount:           1,
			hasSubject:           false,
			subjectInRegistry:    false,
			errorExpected:        false,
		},
		{
			// Basic Artifact type No layers (Error)
			includesArtifactType: true,
			configIsScratch:      true,
			layersAreScratch:     false,
			layercount:           0,
			hasSubject:           false,
			subjectInRegistry:    false,
			errorExpected:        true,
		},
	}
	// Push a Subject
	subjectDesc, err := p.pushOCIImage(ctx, repo, "oci-subject", ociConfig, 2)
	if err != nil {
		return err
	}

	for i, opt := range opts {
		subject := ociimagespec.Descriptor{}
		if opt.hasSubject {
			if opt.subjectInRegistry {
				subject = subjectDesc
			} else {
				uuidStr := uuid.New().String() // Generate a random UUID to make sure subject doesn't exist
				subject = ociimagespec.Descriptor{
					MediaType: ociimagespec.MediaTypeImageIndex,
					Digest:    digest.FromBytes([]byte(uuidStr)),
					Size:      int64(len([]byte(uuidStr))),
				}
			}
		}

		_, err := p.pushOCIArtifact(ctx, &subject, repo, fmt.Sprintf("%s-oci-%d", tagPrefix, i), opt)

		subjectAdded := "Subject Added"
		if !opt.hasSubject {
			subjectAdded = "Subject Missing"
		}

		subjectExists := "Subject Exists"
		if !opt.subjectInRegistry {
			subjectExists = "Subject Missing"
		}

		artifactTypeAdded := "Artifact Type Added"
		if !opt.includesArtifactType {
			artifactTypeAdded = "Artifact Type Missing"
		}

		configType := "Scratch Config"
		if !opt.configIsScratch {
			configType = "Regular Config"
		}

		layerType := "Scratch Layers"
		if !opt.layersAreScratch {
			layerType = "Regular Layers"
		}
		layerType = fmt.Sprintf("%d - %s", opt.layercount, layerType)
		testTitle := fmt.Sprintf("OCI Artifact %d: %s - %s - %s - %s - %s", i, subjectAdded, subjectExists, artifactTypeAdded, configType, layerType)
		p.Logger.Info().Msgf(testTitle)
		if err != nil {
			if opt.errorExpected {
				p.Logger.Info().Msgf("Received Expected Error: %v", err)
				p.Logger.Info().Msgf("Success")
			} else {
				p.Logger.Error().Msgf("Received Unexpected Error: %v", err)
			}
		} else {
			p.Logger.Info().Msgf("Success")
		}
	}
	return nil
}

// Pushes a simple OCI image with @param layercount layers to the registry
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
		MediaType: imagegenConfigMediaType,
		Digest:    digest.FromBytes(configBytes),
		Size:      int64(len(configBytes)),
	}
	err = uploadBytes(ctx, pusher, configDesc, configBytes)
	if err != nil {
		return ociimagespec.Descriptor{}, err
	}

	// upload layers
	var layerDescs []ociimagespec.Descriptor
	for i := 0; i < layercount; i++ {
		layerBytes := []byte(fmt.Sprintf("TestLayer %s %d-at-time %s", tag, i, time.Now()))
		layerDesc := ociimagespec.Descriptor{
			MediaType: ociimagespec.MediaTypeImageLayer,
			Digest:    digest.FromBytes(layerBytes),
			Size:      int64(len(layerBytes)),
		}
		err := uploadBytes(ctx, pusher, layerDesc, layerBytes)
		if err != nil {
			return ociimagespec.Descriptor{}, err
		}
		layerDescs = append(layerDescs, layerDesc)
	}

	ociManifest := ociimagespec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ociimagespec.MediaTypeImageManifest,
		Config:    configDesc,
		Layers:    layerDescs,
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

// Pushes a simple OCI Image Artifact
func (p Proxy) pushOCIArtifact(ctx context.Context, subject *ociimagespec.Descriptor, repo, tag string, opts artifactConstructOptions) (ociimagespec.Descriptor, error) {
	configDescriptor := ociimagespec.ScratchDescriptor
	configBytes := ociimagespec.ScratchDescriptor.Data
	var err error

	if !opts.configIsScratch {
		configBytes, err = json.Marshal(ociConfig)
		if err != nil {
			return ociimagespec.Descriptor{}, err
		}
		configDescriptor.MediaType = imagegenConfigMediaType
		configDescriptor.Digest = digest.FromBytes(configBytes)
		configDescriptor.Size = int64(len(configBytes))
	}

	ref := fmt.Sprintf("%s/%s:%s", p.Options.LoginServer, repo, tag)
	pusher, err := p.resolver.Pusher(ctx, ref)
	// Upload config blob
	if err != nil {
		return ociimagespec.Descriptor{}, err
	}
	err = uploadBytes(ctx, pusher, configDescriptor, configBytes)
	if err != nil {
		return ociimagespec.Descriptor{}, err
	}

	var layerDescs []ociimagespec.Descriptor
	for i := 0; i < opts.layercount; i++ {
		if opts.layersAreScratch {
			// Avoid reuploading the scratch layer if its already been pushed
			if !opts.configIsScratch && i == 0 {
				err = uploadBytes(ctx, pusher, ociimagespec.ScratchDescriptor, ociimagespec.ScratchDescriptor.Data)
				if err != nil {
					return ociimagespec.Descriptor{}, err
				}
			}
			layerDescs = append(layerDescs, ociimagespec.ScratchDescriptor)
		} else {
			layerBytes := []byte(fmt.Sprintf("TestLayer %s %d-at-time %s", tag, i, time.Now()))
			layerDesc := ociimagespec.Descriptor{
				MediaType: ociimagespec.MediaTypeImageLayer,
				Digest:    digest.FromBytes(layerBytes),
				Size:      int64(len(layerBytes)),
			}
			layerDescs = append(layerDescs, layerDesc)
		}
	}

	ociManifest := ociimagespec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ociimagespec.MediaTypeImageManifest,
		Config:    ociimagespec.ScratchDescriptor,
		Layers:    layerDescs,
		Subject:   subject,
	}

	if opts.includesArtifactType {
		ociManifest.ArtifactType = imagegenArtifactType
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
