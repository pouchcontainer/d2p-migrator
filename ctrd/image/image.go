package image

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/remotes/docker"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
)

func defaultRemoteContext() *containerd.RemoteContext {
	return &containerd.RemoteContext{
		Resolver: docker.NewResolver(docker.ResolverOptions{
			Client: http.DefaultClient,
		}),
		Snapshotter: containerd.DefaultSnapshotter,
	}
}

// PullManifest downloads the provided content into containerd's content store
func PullManifest(ctx context.Context, c *containerd.Client, ref string, opts ...containerd.RemoteOpt) error {
	pullCtx := defaultRemoteContext()
	for _, o := range opts {
		if err := o(c, pullCtx); err != nil {
			return err
		}
	}
	store := c.ContentStore()

	ctx, done, err := c.WithLease(ctx)
	if err != nil {
		return err
	}
	defer done()

	name, desc, err := pullCtx.Resolver.Resolve(ctx, ref)
	if err != nil {
		return err
	}
	fetcher, err := pullCtx.Resolver.Fetcher(ctx, name)
	if err != nil {
		return err
	}

	var (
		schema1Converter *Converter
		handler          images.Handler
	)
	if desc.MediaType == images.MediaTypeDockerSchema1Manifest && pullCtx.ConvertSchema1 {
		schema1Converter = NewConverter(store, fetcher)
		handler = images.Handlers(append(pullCtx.BaseHandlers, schema1Converter)...)
	} else {
		handler = images.Handlers(append(pullCtx.BaseHandlers,
			remotes.FetchHandler(store, fetcher),
			childrenHandler(store, platforms.Default()))...,
		)
	}

	if err := images.Dispatch(ctx, handler, desc); err != nil {
		return err
	}
	if schema1Converter != nil {
		desc, err = schema1Converter.Convert(ctx)
		if err != nil {
			return err
		}
	}

	imgrec := images.Image{
		Name:   name,
		Target: desc,
		Labels: pullCtx.Labels,
	}

	is := c.ImageService()
	if _, err := is.Create(ctx, imgrec); err != nil {
		if !errdefs.IsAlreadyExists(err) {
			return err
		}

		_, err := is.Update(ctx, imgrec)
		if err != nil {
			return err
		}
	}

	return nil
}

// childrenHandler decodes well-known manifest types and returns their children.
//
// This is useful for supporting recursive fetch and other use cases where you
// want to do a full walk of resources.
//
// One can also replace this with another implementation to allow descending of
// arbitrary types.
func childrenHandler(provider content.Provider, platform string) images.HandlerFunc {
	return func(ctx context.Context, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
		return children(ctx, provider, desc, platform)
	}
}

// children returns the immediate children of content described by the descriptor.
func children(ctx context.Context, provider content.Provider, desc ocispec.Descriptor, platform string) ([]ocispec.Descriptor, error) {
	var descs []ocispec.Descriptor
	switch desc.MediaType {
	case images.MediaTypeDockerSchema2Manifest, ocispec.MediaTypeImageManifest:
		p, err := content.ReadBlob(ctx, provider, desc.Digest)
		if err != nil {
			return nil, err
		}

		var manifest ocispec.Manifest
		if err := json.Unmarshal(p, &manifest); err != nil {
			return nil, err
		}

		descs = append(descs, manifest.Config)
	case images.MediaTypeDockerSchema2ManifestList, ocispec.MediaTypeImageIndex:
		p, err := content.ReadBlob(ctx, provider, desc.Digest)
		if err != nil {
			return nil, err
		}

		var index ocispec.Index
		if err := json.Unmarshal(p, &index); err != nil {
			return nil, err
		}

		if platform != "" {
			matcher, err := platforms.Parse(platform)
			if err != nil {
				return nil, err
			}

			for _, d := range index.Manifests {
				if d.Platform == nil || matcher.Match(*d.Platform) {
					descs = append(descs, d)
				}
			}
		} else {
			descs = append(descs, index.Manifests...)
		}

	case images.MediaTypeDockerSchema2Layer, images.MediaTypeDockerSchema2LayerGzip,
		images.MediaTypeDockerSchema2LayerForeign, images.MediaTypeDockerSchema2LayerForeignGzip,
		images.MediaTypeDockerSchema2Config, ocispec.MediaTypeImageConfig,
		ocispec.MediaTypeImageLayer, ocispec.MediaTypeImageLayerGzip,
		ocispec.MediaTypeImageLayerNonDistributable, ocispec.MediaTypeImageLayerNonDistributableGzip,
		images.MediaTypeContainerd1Checkpoint, images.MediaTypeContainerd1CheckpointConfig:
		// childless data types.
		return nil, nil
	default:
		logrus.Warnf("encountered unknown type %v; children may not be fetched", desc.MediaType)
	}

	return descs, nil
}

// AddDefaultRegistryIfMissing will add default registry and namespace if missing.
func AddDefaultRegistryIfMissing(ref string, defaultRegistry, defaultNamespace string) string {
	var (
		registry  string
		remainder string
	)

	idx := strings.IndexRune(ref, '/')
	if idx == -1 || !strings.ContainsAny(ref[:idx], ".:") {
		registry, remainder = defaultRegistry, ref
	} else {
		registry, remainder = ref[:idx], ref[idx+1:]
	}

	if registry == defaultRegistry && !strings.ContainsAny(remainder, "/") {
		remainder = defaultNamespace + "/" + remainder
	}
	return registry + "/" + remainder
}
