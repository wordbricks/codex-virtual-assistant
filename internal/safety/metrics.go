package safety

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

const defaultSourceContext = "(none)"

func BrowserActionRecordFromWebStep(projectSlug string, step assistant.WebStep) (assistant.BrowserActionRecord, bool) {
	actionName := strings.TrimSpace(step.ActionName)
	sourceURL := strings.TrimSpace(step.URL)
	targetContext := strings.TrimSpace(firstNonEmpty(step.ActionTarget, step.ActionRef))
	if actionName == "" && sourceURL == "" && targetContext == "" {
		return assistant.BrowserActionRecord{}, false
	}

	actionType, accountStateChanged := classifyBrowserAction(actionName)
	fingerprint := ""
	if payload := payloadForFingerprint(actionType, step.ActionValue); payload != "" {
		fingerprint = fingerprintText(payload)
	}

	return assistant.BrowserActionRecord{
		ProjectSlug:         strings.TrimSpace(projectSlug),
		ActionType:          actionType,
		ActionName:          firstNonEmpty(actionName, "unknown"),
		TargetContext:       targetContext,
		SourceContext:       sourceContextFromURL(sourceURL),
		SourceURL:           sourceURL,
		AccountStateChanged: accountStateChanged,
		TextFingerprint:     fingerprint,
		OccurredAt:          step.OccurredAt.UTC(),
	}, true
}

func ComputeRecentActivityMetrics(records []assistant.BrowserActionRecord, windowStart, windowEnd time.Time) assistant.BrowserRecentActivityMetrics {
	windowStart = windowStart.UTC()
	windowEnd = windowEnd.UTC()
	if windowEnd.Before(windowStart) {
		windowStart, windowEnd = windowEnd, windowStart
	}

	filtered := make([]assistant.BrowserActionRecord, 0, len(records))
	for _, record := range records {
		occurredAt := record.OccurredAt.UTC()
		if occurredAt.Before(windowStart) || occurredAt.After(windowEnd) {
			continue
		}
		record.OccurredAt = occurredAt
		filtered = append(filtered, record)
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].OccurredAt.Before(filtered[j].OccurredAt)
	})

	metrics := assistant.BrowserRecentActivityMetrics{
		WindowStart:      windowStart,
		WindowEnd:        windowEnd,
		TotalActionCount: len(filtered),
	}
	if len(filtered) == 0 {
		return metrics
	}

	sourceCounts := make(map[string]int)
	pairCounts := make(map[string]int)
	fingerprintCounts := make(map[string]int)
	textFingerprintTotal := 0
	maxSourceCount := 0

	for idx, record := range filtered {
		if record.AccountStateChanged {
			metrics.MutatingActionCount++
		}
		if record.ActionType == assistant.BrowserActionTypeReply {
			metrics.ReplyActionCount++
		}

		sourceKey := strings.TrimSpace(record.SourceContext)
		if sourceKey == "" {
			sourceKey = defaultSourceContext
		}
		sourceCounts[sourceKey]++
		if sourceCounts[sourceKey] > maxSourceCount {
			maxSourceCount = sourceCounts[sourceKey]
		}

		if idx > 0 {
			prev := filtered[idx-1]
			pairKey := string(prev.ActionType) + "->" + string(record.ActionType)
			pairCounts[pairKey]++
		}

		fingerprint := strings.TrimSpace(record.TextFingerprint)
		if fingerprint != "" {
			fingerprintCounts[fingerprint]++
			textFingerprintTotal++
		}
	}

	windowHours := windowEnd.Sub(windowStart).Hours()
	if windowHours < 1 {
		windowHours = 1
	}
	metrics.RecentMutationDensity = float64(metrics.MutatingActionCount) / windowHours
	metrics.SourcePathConcentration = float64(maxSourceCount) / float64(metrics.TotalActionCount)

	totalPairs := len(filtered) - 1
	if totalPairs > 0 {
		repeatedPairs := 0
		for _, count := range pairCounts {
			if count > 1 {
				repeatedPairs += count - 1
			}
		}
		metrics.RepeatedActionSequenceScore = float64(repeatedPairs) / float64(totalPairs)
	}

	if textFingerprintTotal > 0 {
		repeatedFingerprints := 0
		for _, count := range fingerprintCounts {
			if count > 1 {
				repeatedFingerprints += count - 1
			}
		}
		metrics.TextReuseRiskScore = float64(repeatedFingerprints) / float64(textFingerprintTotal)
	}

	return metrics
}

func classifyBrowserAction(actionName string) (assistant.BrowserActionType, bool) {
	action := strings.ToLower(strings.TrimSpace(actionName))
	if action == "" {
		return assistant.BrowserActionTypeUnknown, false
	}

	switch {
	case containsAnyToken(action, "open", "goto", "navigate", "visit"):
		return assistant.BrowserActionTypeNavigate, false
	case containsAnyToken(action, "snapshot", "screenshot", "read", "extract", "observe", "scroll", "wait", "hover", "find"):
		return assistant.BrowserActionTypeRead, false
	case containsAnyToken(action, "type", "fill", "input", "edit"):
		return assistant.BrowserActionTypeInput, false
	case containsAnyToken(action, "reply", "comment"):
		return assistant.BrowserActionTypeReply, true
	case containsAnyToken(action, "follow", "connect", "invite", "like", "endorse", "review", "message", "dm", "react"):
		return assistant.BrowserActionTypeEngage, true
	case containsAnyToken(action, "submit", "send", "post", "publish", "save", "apply", "upload", "delete", "confirm"):
		return assistant.BrowserActionTypeSubmit, true
	default:
		return assistant.BrowserActionTypeUnknown, false
	}
}

func payloadForFingerprint(actionType assistant.BrowserActionType, value string) string {
	if actionType != assistant.BrowserActionTypeInput &&
		actionType != assistant.BrowserActionTypeReply &&
		actionType != assistant.BrowserActionTypeEngage &&
		actionType != assistant.BrowserActionTypeSubmit {
		return ""
	}
	return normalizeWhitespace(strings.TrimSpace(value))
}

func sourceContextFromURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	host := strings.TrimSpace(parsed.Host)
	if host == "" {
		return ""
	}
	cleanPath := path.Clean(strings.TrimSpace(parsed.Path))
	if cleanPath == "." || cleanPath == "" {
		cleanPath = "/"
	}
	if !strings.HasPrefix(cleanPath, "/") {
		cleanPath = "/" + cleanPath
	}
	return host + cleanPath
}

func fingerprintText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func normalizeWhitespace(value string) string {
	parts := strings.Fields(value)
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ")
}

func containsAnyToken(value string, tokens ...string) bool {
	for _, token := range tokens {
		if strings.Contains(value, token) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
