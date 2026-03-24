package selector

import (
	"github.com/willmurray/looper/internal/agent"
	"github.com/willmurray/looper/internal/config"
	"github.com/willmurray/looper/internal/detect"
)

// SelectReviewers returns the ordered list of reviewer agent paths to run for
// a given iteration. General reviewer comes first, then specialized in config
// order. Each path appears at most once.
func SelectReviewers(
	reviewers config.Reviewers,
	strategy config.ReviewStrategy,
	metadata map[string]agent.Metadata,
	detected detect.Detection,
	iteration, cycles int,
) []string {
	seen := map[string]bool{}
	var result []string

	add := func(path string) {
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		result = append(result, path)
	}

	switch strategy.Mode {
	case "general-only":
		add(reviewers.General)
		return result

	case "always":
		add(reviewers.General)
		for _, s := range reviewers.Specialized {
			add(s)
		}
		return result

	default: // "smart"
		add(reviewers.General)
		detectedSet := map[string]bool{}
		for _, l := range detected.Languages {
			detectedSet[l] = true
		}
		for _, s := range reviewers.Specialized {
			if includeSpecialized(s, strategy, metadata, detectedSet, iteration, cycles) {
				add(s)
			}
		}
		return result
	}
}

func includeSpecialized(
	path string,
	strategy config.ReviewStrategy,
	metadata map[string]agent.Metadata,
	detectedLangs map[string]bool,
	iteration, cycles int,
) bool {
	every := strategy.SpecializedEvery
	if every > 0 && iteration%every == 0 {
		return true
	}
	if strategy.SpecializedOnCompletion && iteration == cycles {
		return true
	}
	if metadata != nil {
		m := metadata[path]
		for _, lang := range m.Languages {
			if detectedLangs[lang] {
				return true
			}
		}
	}
	return false
}
