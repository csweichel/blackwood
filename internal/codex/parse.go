package codex

import (
	"encoding/json"
	"fmt"
	"strings"
)

func parseJSON(text string, dest any) error {
	candidates := jsonCandidates(text)
	var lastErr error
	for _, candidate := range candidates {
		if err := json.Unmarshal([]byte(candidate), dest); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("no JSON object found")
}

func jsonCandidates(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var candidates []string
	candidates = append(candidates, text)

	if strings.HasPrefix(text, "```") {
		lines := strings.Split(text, "\n")
		if len(lines) >= 3 {
			body := strings.Join(lines[1:len(lines)-1], "\n")
			candidates = append(candidates, strings.TrimSpace(body))
		}
	}

	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}") {
			candidates = append(candidates, line)
		}
	}

	if start := strings.Index(text, "{"); start >= 0 {
		if end := strings.LastIndex(text, "}"); end > start {
			candidates = append(candidates, text[start:end+1])
		}
	}

	seen := make(map[string]bool, len(candidates))
	out := candidates[:0]
	for _, c := range candidates {
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out
}
