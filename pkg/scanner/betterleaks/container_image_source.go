package betterleaks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/fatih/semgroup"
	"github.com/mholt/archives"

	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/pkg/version"

	"github.com/betterleaks/betterleaks/sources"
	"github.com/betterleaks/betterleaks/sources/scm"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/pkg/blobinfocache"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"

	imagespecv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	AttrContainerAuthorEmail = "oci.author.email"
	AttrContainerAuthorName  = "oci.author.name"
	AttrContainerDigest      = "oci.digest"
)

type ContainerImage struct {
	Arch            string
	Depth           int
	Exclusions      []string
	MaxArchiveDepth int
	ShouldSkip      sources.SkipFunc
	RawImageRef     string
	Sema            *semgroup.Group
	Since           *time.Time
	Platform        scm.Platform
	path            string
}

var authorRe = regexp.MustCompile(`^(.+?)\s+<([^>]+)`)

type seekReaderAt interface {
	io.ReaderAt
	io.Seeker
}

func (s *ContainerImage) Fragments(ctx context.Context, yield sources.FragmentsFunc) error {
	sysCtx := &types.SystemContext{
		DockerRegistryUserAgent: version.GlobalUserAgent,
	}

	imageRef, err := alltransports.ParseImageName(s.RawImageRef)
	if err != nil {
		logger.Debug("error parsing image reference %q: %v adding transport and trying again", s.RawImageRef, err)
		imageRef, err = alltransports.ParseImageName("docker://" + s.RawImageRef)
		if err != nil {
			return fmt.Errorf("could not parse image reference: %v image=%q", err, s.RawImageRef)
		}
	}

	imageSource, err := imageRef.NewImageSource(ctx, sysCtx)
	if err != nil {
		return fmt.Errorf("could not create image source: %v image=%q", err, s.RawImageRef)
	}

	defer (func() {
		if err := imageSource.Close(); err != nil {
			logger.Debug("error closing image source: %v image=%q", err, s.RawImageRef)
		}
	})()

	logger.Debug("fetching manifest: image=%q", s.RawImageRef)
	rawManifest, manifestMIMEType, err := imageSource.GetManifest(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not fetch manifest: %v", err)
	}

	var indexManifest *manifest.Schema2List

	switch manifestMIMEType {
	case imagespecv1.MediaTypeImageIndex:
		var oci1Index manifest.OCI1Index
		err := json.Unmarshal(rawManifest, &oci1Index)
		if err != nil {
			return fmt.Errorf("could not unmarshal manifest: %v", err)
		}
		indexManifest, err = oci1Index.ToSchema2List()
		if err != nil {
			return fmt.Errorf("could not convert oci index manifest to schema2list: %v", err)
		}
	case manifest.DockerV2ListMediaType:
		var schema2List manifest.Schema2List
		err := json.Unmarshal(rawManifest, &schema2List)
		if err != nil {
			return fmt.Errorf("could not unmarshal manifest: %v", err)
		}
		indexManifest = &schema2List
	}

	if indexManifest != nil && len(indexManifest.Manifests) > 0 {
		for _, m := range indexManifest.Manifests {
			digest := m.Digest.String()
			var rawImageRef string
			if len(s.Arch) > 0 {
				if m.Platform.Architecture == s.Arch {
					rawImageRef = imageSource.Reference().DockerReference().Name() + "@" + digest
				}
			} else {
				rawImageRef = imageSource.Reference().DockerReference().Name() + "@" + digest
			}

			if len(rawImageRef) > 0 {
				containerImage := *s
				containerImage.RawImageRef = imageSource.Reference().Transport().Name() + "://" + rawImageRef
				containerImage.path = filepath.Join(s.path, "manifests", digest)

				if err := containerImage.Fragments(ctx, yield); err != nil {
					return err
				}
			}
		}

		return nil
	}

	image, err := imageRef.NewImage(ctx, sysCtx)
	if err != nil {
		return fmt.Errorf("could not load image to retrieve labels: %v", err)
	}

	defer (func() {
		if err := image.Close(); err != nil {
			logger.Debug("error closing image: %v image=%q", err, s.RawImageRef)
		}
	})()

	imageManifest, err := manifest.FromBlob(rawManifest, manifestMIMEType)
	if err != nil {
		return fmt.Errorf("could not parse manifest: %v image=%q", err, s.RawImageRef)
	}

	ociConfig, err := image.OCIConfig(ctx)
	if err != nil {
		return fmt.Errorf("could not get OCI config: %v image=%q", err, s.RawImageRef)
	}

	configHistories := make([]imagespecv1.History, 0, len(ociConfig.History))
	for _, h := range ociConfig.History {
		if h.EmptyLayer {
			continue
		}
		configHistories = append(configHistories, h)
	}

	findingAttrs := s.findingAttrsFromConfig(ociConfig)
	findingAttrs[AttrContainerDigest] = imageManifest.ConfigInfo().Digest.String()
	manifestJSON := &JSON{
		MaxArchiveDepth: s.MaxArchiveDepth,
		Path:            filepath.Join(s.path, "manifest"),
		RawMessage:      rawManifest,
		ShouldSkip:      s.ShouldSkip,
	}

	err = manifestJSON.Fragments(ctx, yieldWithFindingAttrs(findingAttrs, yield))
	if err != nil {
		return err
	}

	var currentDepth int

	cache := blobinfocache.DefaultCache(sysCtx)
	layerInfos := imageManifest.LayerInfos()
	checkSince := s.Since != nil && len(layerInfos) == len(configHistories)

	for i, layerInfo := range layerInfos {
		if layerInfo.EmptyLayer {
			logger.Debug("skipping empty layer: digest=%q", layerInfo.Digest)
			continue
		}

		currentDepth++
		if s.Depth > 0 && s.Depth < currentDepth {
			logger.Debug("layer depth exceeded: digest=%q max_depth=%d", layerInfo.Digest, s.Depth)
			break
		}

		if checkSince {
			if history := configHistories[i]; history.Created != nil && history.Created.Before(*s.Since) {
				logger.Debug("skipping layer older than provided date: digest=%q create=%q", layerInfo.Digest, history.Created.Format("2006-01-02"))
				continue
			}
		}

		if slices.Contains(s.Exclusions, layerInfo.Digest.Hex()) {
			logger.Debug("skipping layer in exclusions list: digest=%q", layerInfo.Digest)
			continue
		}

		digest := layerInfo.Digest.String()
		layerFindingAttrs := make(map[string]string, len(findingAttrs))
		maps.Copy(layerFindingAttrs, findingAttrs)
		layerFindingAttrs[AttrContainerDigest] = digest
		enrichedYield := yieldWithFindingAttrs(layerFindingAttrs, yield)

		logger.Debug("downloading container layer blob: digest=%q", digest)
		blobReader, blobSize, err := imageSource.GetBlob(ctx, layerInfo.BlobInfo, cache)
		logger.Debug("container layer blob size: digest=%q size=%d", digest, blobSize)
		if err != nil {
			logger.Error("could not download layer blob: %v", err)
			return err
		}

		format, stream, err := archives.Identify(ctx, "", blobReader)
		if err == nil && format != nil {
			if extractor, ok := format.(archives.Extractor); ok {
				s.extractorFragments(ctx, extractor, digest, stream, enrichedYield)
				continue
			} else if decompressor, ok := format.(archives.Decompressor); ok {
				s.decompressorFragments(ctx, decompressor, digest, stream, enrichedYield)
				continue
			}
		}

		file := &sources.File{
			Content:         stream,
			MaxArchiveDepth: s.MaxArchiveDepth - 1,
			Path:            filepath.Join(s.path, "layers", digest),
			ShouldSkip:      s.ShouldSkip,
		}

		err = file.Fragments(ctx, enrichedYield)

		if closeErr := blobReader.Close(); closeErr != nil {
			logger.Debug("error closing blob reader: %v digest=%q", closeErr, digest)
		}

		if err != nil {
			return err
		}
	}

	return nil
}

func (s *ContainerImage) extractorFragments(ctx context.Context, extractor archives.Extractor, digest string, reader io.Reader, yield sources.FragmentsFunc) {
	if _, isSeekReaderAt := reader.(seekReaderAt); !isSeekReaderAt {
		switch extractor.(type) {
		case archives.SevenZip, archives.Zip:
			tmpfile, err := os.CreateTemp("", "leaktk-archive-")
			tmpfilePath := filepath.Clean(tmpfile.Name())
			if err != nil {
				logger.Error("could not create tmp file for container layer blob: %v digest=%q", err, digest)
				return
			}
			defer func() {
				_ = tmpfile.Close()
				_ = os.Remove(tmpfilePath)
			}()

			_, err = io.Copy(tmpfile, reader)
			if err != nil {
				logger.Error("could not copy container layer blob: %v digest=%q", err, digest)
				return
			}

			reader = tmpfile
		}
	}

	err := extractor.Extract(ctx, reader, func(_ context.Context, d archives.FileInfo) error {
		path := filepath.Clean(d.NameInArchive)
		if !d.Mode().IsRegular() {
			logger.Trace("skipping non-regular file: path=%q digest=%q", path, digest)
			return nil
		}
		if s.ShouldSkip != nil && shouldSkipPath(s.ShouldSkip, path) {
			logger.Debug("skipping file: global allowlist: path=%q digest=%q", path, digest)
			return nil
		}
		innerReader, err := d.Open()
		if err != nil {
			logger.Error("could not open container layer blob inner file: %v path=%q digest=%q", err, path, digest)
			return nil
		}

		file := &sources.File{
			Content:         innerReader,
			MaxArchiveDepth: s.MaxArchiveDepth - 1,
			Path:            filepath.Join(s.path, "layers", digest) + sources.InnerPathSeparator + path,
			ShouldSkip:      s.ShouldSkip,
		}

		if err := file.Fragments(ctx, yield); err != nil {
			logger.Error("error generating file fragments: %v path=%q digest=%q", err, path, digest)
		}
		if err := innerReader.Close(); err != nil {
			logger.Debug("error closing inner reader: %v path=%q digest=%q", err, path, digest)
		}

		return nil
	})

	if err != nil {
		logger.Error("error generating file fragments: %v path=%q digest=%q", err, filepath.Join(s.path, "layers", digest), digest)
	}
}

func (s *ContainerImage) decompressorFragments(ctx context.Context, decompressor archives.Decompressor, digest string, reader io.Reader, yield sources.FragmentsFunc) {
	innerReader, err := decompressor.OpenReader(reader)
	if err != nil {
		logger.Error("could not read compressed container layer blob: %v digest=%q", err, digest)
		return
	}

	file := &sources.File{
		Content:         innerReader,
		MaxArchiveDepth: s.MaxArchiveDepth - 1,
		Path:            filepath.Join(s.path, "layers", digest),
		ShouldSkip:      s.ShouldSkip,
	}

	if err := file.Fragments(ctx, yield); err != nil {
		logger.Error("error generating file fragments: %v path=%q digest=%q", err, file.Path, digest)
	}
}

func yieldWithFindingAttrs(findingAttrs map[string]string, yield sources.FragmentsFunc) sources.FragmentsFunc {
	return func(fragment sources.Fragment, err error) error {
		if err == nil {
			maps.Copy(fragment.Attributes, findingAttrs)
		}
		return yield(fragment, err)
	}
}

func (s *ContainerImage) findingAttrsFromConfig(image *imagespecv1.Image) map[string]string {
	findingAttrs := make(map[string]string)
	labels := image.Config.Labels

	if labelValue, ok := labels["email"]; ok {
		findingAttrs[AttrContainerAuthorEmail] = strings.TrimSpace(labelValue)
	}

	for _, labelName := range []string{
		"org.opencontainers.image.authors",
		"author",
		"org.opencontainers.image.maintainers",
		"maintainer",
	} {
		if labelValue, ok := labels[labelName]; ok {
			if match := authorRe.FindStringSubmatch(labelValue); match != nil {
				findingAttrs[AttrContainerAuthorName] = strings.TrimSpace(match[1])
				findingAttrs[AttrContainerAuthorEmail] = strings.TrimSpace(match[2])
				return findingAttrs
			}
			findingAttrs[AttrContainerAuthorName] = strings.TrimSpace(labelValue)
			return findingAttrs
		}
	}

	return findingAttrs
}

// shouldSkipPath checks a path against the skip callback.
// Also handles the Windows forward-slash path normalization workaround.
func shouldSkipPath(skip sources.SkipFunc, path string) bool {
	if skip == nil {
		logger.Trace("not skipping path because skip func is nil: path=%q", path)
		return false
	}

	return skip(map[string]string{sources.AttrPath: filepath.ToSlash(path)})
}
