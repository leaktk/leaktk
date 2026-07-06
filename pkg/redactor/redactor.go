package redactor

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/betterleaks/betterleaks/detect"

	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/proto"
	"github.com/leaktk/leaktk/pkg/scanner/betterleaks"
)

type Redactor struct {
	Config     *config.Config
	pending    string
	pendingRaw string
	detector   *detect.Detector
}

func NewRedactor(cfg *config.Config) (*Redactor, error) {
	rawConfig, err := os.ReadFile(cfg.Scanner.Patterns.Gitleaks.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("could not read gitleaks config: %w", err)
	}

	blCfg, err := betterleaks.ParseConfig(string(rawConfig))
	if err != nil {
		return nil, fmt.Errorf("could not load scanner config: %w", err)
	}

	detector := detect.NewDetectorContext(context.Background(), *blCfg)
	detector.FollowSymlinks = false
	detector.IgnoreGitleaksAllow = false
	detector.MaxArchiveDepth = cfg.Scanner.MaxArchiveDepth
	detector.MaxDecodeDepth = cfg.Scanner.MaxDecodeDepth
	detector.MaxTargetMegaBytes = 0
	detector.NoColor = true
	detector.Redact = 0
	detector.Verbose = false

	return &Redactor{
		Config:   cfg,
		detector: detector,
	}, nil
}

func (r *Redactor) RedactText(resource string, response *proto.Response) (string, error) {
	mark := r.Config.Redactor.RedactionMark
	word := r.Config.Redactor.RedactionWord

	if len(mark) == 0 && len(word) == 0 {
		mark = "*"
	}

	combinedRaw := r.pendingRaw + resource

	redacted, err := r.scanChunk(context.Background(), r.detector, combinedRaw, mark, word)
	if err != nil {
		return "", fmt.Errorf("unable to scan chunk: %w", err)
	}

	split := max(0, len(redacted)-len(resource))
	r.pending = redacted[split:]
	r.pendingRaw = combinedRaw[max(0, len(combinedRaw)-len(resource)):]

	return redacted[:split], nil
}

func (r *Redactor) Flush() string {
	out := r.pending
	r.pending = ""
	return out
}

func (r *Redactor) scanChunk(ctx context.Context, detector *detect.Detector, chunk string, mark string, word string) (string, error) {
	findings, err := betterleaks.ScanReader(ctx, detector, strings.NewReader(chunk))
	if err != nil {
		return "", fmt.Errorf("betterleaks error: %w", err)
	}

	if len(findings) == 0 {
		return chunk, nil
	}

	sort.Slice(findings, func(i, j int) bool {
		return len(findings[i].Secret) > len(findings[j].Secret)
	})

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
