package gitleaks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/fatih/semgroup"
	"github.com/mholt/archives"

	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/version"

	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/pkg/blobinfocache"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	"github.com/zricethezav/gitleaks/v8/config"
	"github.com/zricethezav/gitleaks/v8/sources"

	imagespecv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type ContainerImage struct {
	Arch            string
	CloneTimeout    time.Duration
	Config          *config.Config
	Depth           int
	Exclusions      []string
	MaxArchiveDepth int
	RawImageRef     string
	Sema            *semgroup.Group
	Since           *time.Time
	path            string
}

var authorRe = regexp.MustCompile(`(?i)^(.+?)\s+<([^>]+)`)

type seekReaderAt interface {
	io.ReaderAt
	io.Seeker
}

func (s *ContainerImage) Fragments(ctx context.Context, yield sources.FragmentsFunc) error {
	if s.CloneTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.CloneTimeout)
		defer cancel()
	}

	sysCtx := &types.SystemContext{
		DockerRegistryUserAgent: version.GlobalUserAgent,
	}

	imageRef, err := alltransports.ParseImageName(s.RawImageRef)
	if err != nil {
		return fmt.Errorf("could not parse image reference: %v", err)
	}

	imageSource, err := imageRef.NewImageSource(ctx, sysCtx)
	if err != nil {
		return fmt.Errorf("could not create image source: %v", err)
	}

	defer (func() {
		if err := imageSource.Close(); err != nil {
			logger.Debug("error closing image source: %v image=%q", err, s.RawImageRef)
		}
	})()

	rawManifest, manifestMIMEType, err := imageSource.GetManifest(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not fetch manifest: %v", err)
	}

	if manifestMIMEType == manifest.DockerV2ListMediaType {
		var indexManifest manifest.Schema2List
		var wg sync.WaitGroup

		if err := json.Unmarshal(rawManifest, &indexManifest); err != nil {
			return fmt.Errorf("could not unmarshal manifest: %v", err)
		}

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
				wg.Add(1)
				s.Sema.Go(func() error {
					containerImage := &ContainerImage{
						Arch:         s.Arch,
						CloneTimeout: s.CloneTimeout,
						Depth:        s.Depth,
						Exclusions:   s.Exclusions,
						RawImageRef:  rawImageRef,
						Sema:         s.Sema,
						Since:        s.Since,
						path:         filepath.Join(s.path, "manifests", digest),
					}

					defer wg.Done()

					return containerImage.Fragments(ctx, yield)
				})
			}
		}

		wg.Wait()

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
		return fmt.Errorf("could not parse manifest: %v", err)
	}

	ociConfig, err := image.OCIConfig(ctx)
	if err != nil {
		return fmt.Errorf("could not get OCI config: %w", err)
	}

	commitInfo := commitInfoFromConfig(ociConfig)
	commitInfo.SHA = imageManifest.ConfigInfo().Digest.String()

	s.Sema.Go(func() error {
		manifestJSON := &JSON{
			Config:          s.Config,
			MaxArchiveDepth: s.MaxArchiveDepth,
			Path:            filepath.Join(s.path, "manifest"),
			RawMessage:      rawManifest,
		}

		return manifestJSON.Fragments(ctx, yieldWithCommitInfo(commitInfo, yield))
	})

	var wg sync.WaitGroup
	var currentDepth int

	cache := blobinfocache.DefaultCache(sysCtx)
	layerInfos := imageManifest.LayerInfos()
	for i, layerInfo := range layerInfos {
		layerCommitInfo := *commitInfo
		layerCommitInfo.SHA = layerInfo.Digest.String()

		if layerInfo.EmptyLayer {
			logger.Debug("skipping empty layer: digest=%q", layerInfo.Digest)

			continue
		}

		currentDepth++
		if s.Depth > 0 && s.Depth < currentDepth {
			logger.Debug(
				"layer depth exceeded: digest=%q max_depth=%d",
				layerInfo.Digest,
				s.Depth,
			)

			break
		}

		if layerHistory := ociConfig.History[i]; s.Since != nil && layerHistory.Created != nil {
			if layerHistory.Created.Before(*s.Since) {
				logger.Debug(
					"skipping layer older than provided date: digest=%q created=%q",
					layerInfo.Digest,
					layerHistory.Created.Format("2006-01-02"),
				)

				continue
			}
		}

		if slices.Contains(s.Exclusions, layerInfo.Digest.Hex()) {
			logger.Debug("skiping layer in exclusions list: digest=%q", layerInfo.Digest)

			continue
		}

		wg.Add(1)
		s.Sema.Go(func() error {
			defer wg.Done()
			yield = yieldWithCommitInfo(&layerCommitInfo, yield)
			digest := layerInfo.Digest.String()

			logger.Debug("downloading container layer blob: digest=%q", digest)
			blobReader, blobSize, err := imageSource.GetBlob(ctx, layerInfo.BlobInfo, cache)
			logger.Debug("container layer blob size: digest=%q size=%d", digest, blobSize)
			if err != nil {
				logger.Error("could not download layer blob: %v", err)

				return err
			}

			defer (func() {
				if err := blobReader.Close(); err != nil {
					logger.Debug("error closing blob reader: %v digest=%q", err, digest)
				}
			})()

			format, stream, err := archives.Identify(ctx, "", blobReader)
			if err == nil && format != nil {
				if extractor, ok := format.(archives.Extractor); ok {
					return s.extractorFragments(ctx, extractor, digest, stream, yield)
				}
				if decompressor, ok := format.(archives.Decompressor); ok {
					return s.decompressorFragments(ctx, decompressor, digest, stream, yield)
				}
			}

			file := &sources.File{
				Content:         stream,
				MaxArchiveDepth: s.MaxArchiveDepth - 1,
				Path:            filepath.Join(s.path, "layers", digest),
			}

			return file.Fragments(ctx, yield)
		})
	}

	wg.Wait()

	return nil
}

func (s *ContainerImage) extractorFragments(ctx context.Context, extractor archives.Extractor, digest string, reader io.Reader, yield sources.FragmentsFunc) error {
	if _, isSeekReaderAt := reader.(seekReaderAt); !isSeekReaderAt {
		switch extractor.(type) {
		case archives.SevenZip, archives.Zip:
			tmpfile, err := os.CreateTemp("", "gitleaks-archive-")
			if err != nil {
				logger.Error("could not create tmp file for container layer blob: digest=%q", digest)

				return nil
			}
			defer func() {
				_ = tmpfile.Close()
				_ = os.Remove(tmpfile.Name())
			}()

			_, err = io.Copy(tmpfile, reader)
			if err != nil {
				logger.Error("could not copy container layer blob: digest=%q", digest)

				return nil
			}

			reader = tmpfile
		}
	}

	return extractor.Extract(ctx, reader, func(_ context.Context, d archives.FileInfo) error {
		if d.IsDir() {
			return nil
		}

		path := filepath.Clean(d.NameInArchive)
		if s.Config != nil && shouldSkipPath(s.Config, path) {
			logger.Debug("skipping file: global allowlist: path=%q digest=%q", path, digest)

			return nil
		}

		innerReader, err := d.Open()
		if err != nil {
			logger.Error("could not open container layer blob inner file: path=%q digest=%q", path, digest)

			return nil
		}

		defer (func() {
			if err := innerReader.Close(); err != nil {
				logger.Debug("error closing inner reader: %v path=%q digest=%q", err, path, digest)
			}
		})()

		file := &sources.File{
			Content:         innerReader,
			Path:            filepath.Join(s.path, "layers", digest) + sources.InnerPathSeparator + path,
			MaxArchiveDepth: s.MaxArchiveDepth - 1,
		}

		return file.Fragments(ctx, yield)
	})
}

func (s *ContainerImage) decompressorFragments(ctx context.Context, decompressor archives.Decompressor, digest string, reader io.Reader, yield sources.FragmentsFunc) error {
	innerReader, err := decompressor.OpenReader(reader)
	if err != nil {
		logger.Error("could not read compressed container layer blob: digest=%q", digest)

		return nil
	}

	file := &sources.File{
		Content:         innerReader,
		MaxArchiveDepth: s.MaxArchiveDepth - 1,
		Path:            filepath.Join(s.path, "layers", digest),
	}

	return file.Fragments(ctx, yield)
}

func yieldWithCommitInfo(commitInfo *sources.CommitInfo, yield sources.FragmentsFunc) sources.FragmentsFunc {
	return func(fragment sources.Fragment, err error) error {
		if err == nil {
			fragment.CommitInfo = commitInfo
			fragment.CommitSHA = commitInfo.SHA
		}

		return yield(fragment, err)
	}
}

func commitInfoFromConfig(image *imagespecv1.Image) *sources.CommitInfo {
	commitInfo := &sources.CommitInfo{}
	labels := image.Config.Labels

	if labelValue, ok := labels["email"]; ok {
		commitInfo.AuthorEmail = strings.TrimSpace(labelValue)
	}

	for _, labelName := range []string{
		"org.opencontainers.image.authors",
		"author",
		"org.opencontainers.image.maintainers",
		"maintainer",
	} {
		if labelValue, ok := labels[labelName]; ok {
			if match := authorRe.FindStringSubmatch(labelValue); match != nil {
				commitInfo.AuthorName = match[0]
				commitInfo.AuthorEmail = match[1]

				return commitInfo
			}
			commitInfo.AuthorName = strings.TrimSpace(labelValue)

			return commitInfo
		}
	}

	return commitInfo
}

func shouldSkipPath(cfg *config.Config, path string) bool {
	if cfg == nil {
		logger.Debug("not skipping path because config is nil: path=%q", path)

		return false
	}

	for _, a := range cfg.Allowlists {
		if a.PathAllowed(filepath.ToSlash(path)) {
			return true
		}
	}

	return false
}
