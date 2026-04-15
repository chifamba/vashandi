package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type FeedbackRedactionState struct {
	RedactedFields  map[string]bool
	TruncatedFields map[string]bool
	OmittedFields   map[string]bool
	Notes           map[string]bool
	Counts          map[string]int
}

func NewFeedbackRedactionState() *FeedbackRedactionState {
	return &FeedbackRedactionState{
		RedactedFields:  make(map[string]bool),
		TruncatedFields: make(map[string]bool),
		OmittedFields:   make(map[string]bool),
		Notes:           make(map[string]bool),
		Counts:          make(map[string]int),
	}
}

type RedactionPattern struct {
	Kind        string
	Regex       *regexp.Regexp
	Replacement func(match string, groups []string) string
}

var freeTextPatterns = []RedactionPattern{
	{
		Kind:  "pem_block",
		Regex: regexp.MustCompile(`-----BEGIN [^-]+-----[\s\S]+?-----END [^-]+-----`),
		Replacement: func(m string, g []string) string {
			return "[REDACTED_PEM_BLOCK]"
		},
	},
	{
		Kind:  "bearer_token",
		Regex: regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9._~+/-]+=*`),
		Replacement: func(m string, g []string) string {
			return "Bearer [REDACTED_TOKEN]"
		},
	},
	{
		Kind:  "secret_assignment",
		// Prevent double-matching "Authorization: Bearer token" by making sure the secret assignment doesn't match "Authorization=[REDACTED_TOKEN]" or similar if Bearer already ran.
		// A simpler approach: just run secret assignment after bearer and ensure it doesn't double-replace. We can adjust the regex or the test expectation.
		Regex: regexp.MustCompile(`(?i)\b(api[-_]?key|access[-_]?token|auth(?:_?token)?|authorization|bearer|secret|passwd|password|credential|jwt|private[-_]?key|cookie|connectionstring)\s*[:=]\s*([^\s,;]+)`),
		Replacement: func(m string, g []string) string {
			// If it already looks like a redaction tag from a previous step, skip it
			if strings.Contains(g[2], "[REDACTED") {
				return m
			}
			return fmt.Sprintf("%s=[REDACTED]", g[1])
		},
	},
	{
		Kind:  "github_token",
		Regex: regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9_]{20,}\b`),
		Replacement: func(m string, g []string) string {
			return "[REDACTED_GITHUB_TOKEN]"
		},
	},
	{
		Kind:  "provider_api_key",
		Regex: regexp.MustCompile(`\bsk-(?:ant-)?[A-Za-z0-9_-]{12,}\b`),
		Replacement: func(m string, g []string) string {
			return "[REDACTED_API_KEY]"
		},
	},
	{
		Kind:  "jwt",
		Regex: regexp.MustCompile(`\b[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+(?:\.[A-Za-z0-9_-]+)?\b`),
		Replacement: func(m string, g []string) string {
			return "[REDACTED_JWT]"
		},
	},
	{
		Kind:  "dsn",
		Regex: regexp.MustCompile(`(?i)\b(?:postgres(?:ql)?|mysql|mongodb(?:\+srv)?|redis|amqp|kafka|nats|mssql):\/\/[^\s<>'")]+`),
		Replacement: func(m string, g []string) string {
			return "[REDACTED_CONNECTION_STRING]"
		},
	},
	{
		Kind:  "email",
		Regex: regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`),
		Replacement: func(m string, g []string) string {
			return "[REDACTED_EMAIL]"
		},
	},
	{
		Kind:  "phone",
		Regex: regexp.MustCompile(`\+?\b\d[\d ()-]{7,}\d\b`),
		Replacement: func(m string, g []string) string {
			return "[REDACTED_PHONE]"
		},
	},
}

func increment(state *FeedbackRedactionState, kind string, count int) {
	if count <= 0 {
		return
	}
	state.Counts[kind] += count
}

func recordField(state *FeedbackRedactionState, fieldPath string) {
	if strings.TrimSpace(fieldPath) == "" {
		return
	}
	state.RedactedFields[fieldPath] = true
}

func applyPattern(input string, pattern RedactionPattern) (string, int) {
	matches := pattern.Regex.FindAllStringSubmatch(input, -1)
	count := len(matches)
	if count == 0 {
		return input, 0
	}

	output := pattern.Regex.ReplaceAllStringFunc(input, func(m string) string {
		groups := pattern.Regex.FindStringSubmatch(m)
		// For patterns that might return unmodified strings (like the secret_assignment check above)
		// we only count it as a match if the output actually changed.
		res := pattern.Replacement(m, groups)
		if res == m {
			count--
		}
		return res
	})

	return output, count
}

func RedactCurrentUserText(input string) string {
	return input
}

func SanitizeFeedbackText(input string, state *FeedbackRedactionState, fieldPath string, maxLength int) string {
	output := RedactCurrentUserText(input)
	if output != input {
		recordField(state, fieldPath)
		increment(state, "current_user", 1)
	}

	for _, pattern := range freeTextPatterns {
		resOutput, matches := applyPattern(output, pattern)
		if matches > 0 {
			output = resOutput
			recordField(state, fieldPath)
			increment(state, pattern.Kind, matches)
		}
	}

	if len(output) > maxLength {
		output = output[:maxLength-1] + "..."
		state.TruncatedFields[fieldPath] = true
	}

	return output
}

func FinalizeFeedbackRedactionSummary(state *FeedbackRedactionState) map[string]interface{} {
	redactedFields := make([]string, 0, len(state.RedactedFields))
	for k := range state.RedactedFields {
		redactedFields = append(redactedFields, k)
	}
	sort.Strings(redactedFields)

	truncatedFields := make([]string, 0, len(state.TruncatedFields))
	for k := range state.TruncatedFields {
		truncatedFields = append(truncatedFields, k)
	}
	sort.Strings(truncatedFields)

	omittedFields := make([]string, 0, len(state.OmittedFields))
	for k := range state.OmittedFields {
		omittedFields = append(omittedFields, k)
	}
	sort.Strings(omittedFields)

	notes := make([]string, 0, len(state.Notes))
	for k := range state.Notes {
		notes = append(notes, k)
	}
	sort.Strings(notes)

	return map[string]interface{}{
		"strategy":        "deterministic_feedback_v2",
		"redactedFields":  redactedFields,
		"truncatedFields": truncatedFields,
		"omittedFields":   omittedFields,
		"notes":           notes,
		"counts":          state.Counts,
	}
}

func StableStringify(value interface{}) string {
	if value == nil {
		return "null"
	}

	switch v := value.(type) {
	case []interface{}:
		var parts []string
		for _, item := range v {
			parts = append(parts, StableStringify(item))
		}
		return "[" + strings.Join(parts, ",") + "]"
	case map[string]interface{}:
		var keys []string
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		var parts []string
		for _, k := range keys {
			keyJSON, _ := json.Marshal(k)
			parts = append(parts, fmt.Sprintf("%s:%s", string(keyJSON), StableStringify(v[k])))
		}
		return "{" + strings.Join(parts, ",") + "}"
	case float64:
		b, _ := json.Marshal(v)
		return string(b)
	case int: // For tests primarily
		b, _ := json.Marshal(v)
		return string(b)
	case int64, int32, uint, uint64, uint32:
		return fmt.Sprintf("%d", v)
	case string:
		b, _ := json.Marshal(v)
		return string(b)
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func SHA256Digest(value interface{}) string {
	str := StableStringify(value)
	hash := sha256.Sum256([]byte(str))
	return hex.EncodeToString(hash[:])
}
