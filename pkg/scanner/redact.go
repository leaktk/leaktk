package scanner

import (
	"context"
	"io"
	"strings"
	"time"
	"fmt"

	"github.com/betterleaks/betterleaks/detect"
	"github.com/leaktk/leaktk/pkg/scanner/betterleaks"
	"github.com/leaktk/leaktk/pkg/logger"
)

func (s *Scanner) RedactStream(
	ctx context.Context,
	r io.Reader,
	w io.Writer,
	kind string,
	redactionMark string,
	redactionWord string,
) error {
	if len(redactionMark) == 0 && len(redactionWord) == 0 {
		redactionMark = "*"
	}

	cfg, err := s.patterns.Gitleaks(ctx)
	if err != nil {
		return fmt.Errorf("scan failed: could load scanner config: %v", err)
	}

	detector := detect.NewDetectorContext(ctx, *cfg)
	detector.FollowSymlinks = false
	detector.IgnoreGitleaksAllow = false
	detector.MaxArchiveDepth = s.maxArchiveDepth
	detector.MaxDecodeDepth = s.maxDecodeDepth
	detector.MaxTargetMegaBytes = 0
	detector.NoColor = true
	detector.Redact = 0
	detector.Verbose = false

	buf := make([]byte, 2048)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			n, err := r.Read(buf)
			logger.Info("About to scan")
			if n > 0 {
				logger.Info("2")
				chunk := string(buf[:n])
				sanitizedChunk, err := s.scanChunk(ctx, detector, chunk, redactionMark, redactionWord)
				if err != nil {
					return fmt.Errorf("Unable to scan chunk: %w", err)
				}
				if _, writeErr := w.Write([]byte(sanitizedChunk)); writeErr != nil {
					return writeErr
				}
			}

			if err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}

			time.Sleep(128 * time.Millisecond)
		}
	}
}

func (s *Scanner) scanChunk(ctx context.Context, detector *detect.Detector, chunk string, mark string, word string) (string, error) {
	logger.Info("Scanning")
	findings, err := betterleaks.ScanReader(ctx, detector, strings.NewReader(chunk))
	if err != nil {
		return "", fmt.Errorf("Betterleaks error: %w", err)
	}

	if len(findings) == 0 {
		return chunk, nil
	}

	for _, finding := range findings {
		if len(finding.Secret) == 0 {
			continue
		}

		if word != "" {
			chunk = strings.ReplaceAll(chunk, finding.Secret, word)
		} else {
			mask := strings.Repeat(mark, len(finding.Secret))
			chunk = strings.ReplaceAll(chunk, finding.Secret, mask)
		}
	}

	return chunk, nil
}
