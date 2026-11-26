// Package knowledge contains deterministic validation shared by extraction and review.
package knowledge

import (
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
)

type Extraction struct {
	ShouldSave         bool     `json:"should_save"`
	Title              string   `json:"title"`
	Summary            string   `json:"summary"`
	Content            string   `json:"content"`
	Action             string   `json:"action"`
	TargetDocumentID   string   `json:"target_document_id"`
	NavID              string   `json:"nav_id"`
	ParentID           string   `json:"parent_id"`
	KnowledgeType      string   `json:"knowledge_type"`
	VerificationStatus string   `json:"verification_status"`
	RiskLevel          string   `json:"risk_level"`
	Reason             string   `json:"reason"`
	SourceMessageIDs   []string `json:"source_message_ids"`
	OldText            string   `json:"old_text"`
	NewText            string   `json:"new_text"`
}

var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)-----BEGIN [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)\b(?:sk-[a-z0-9_-]{12,}|AKIA[A-Z0-9]{16}|gh[pousr]_[a-z0-9]{20,})`),
	regexp.MustCompile(`(?i)(?:password|passwd|api[_ -]?key|access[_ -]?token|secret|密码|口令|密钥)\s*[=:：]\s*["']?[^\s"'<>]{4,}`),
	regexp.MustCompile(`(?i)(?:postgres(?:ql)?|mysql|redis)://[^\s:/]+:[^\s@]+@`),
	regexp.MustCompile(`(?i)\bBearer\s+[a-z0-9._~+/-]{12,}`),
	regexp.MustCompile(`\b1[3-9][0-9]{9}\b`),
	regexp.MustCompile(`\b[0-9]{17}[0-9Xx]\b`),
}

func Sensitive(text string) bool {
	for _, p := range sensitivePatterns {
		if p.MatchString(text) {
			return true
		}
	}
	return false
}

func Decode(raw string, target any) error {
	if len(raw) > 1024*1024 {
		return errors.New("model output too large")
	}
	d := json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("trailing model output")
	}
	return nil
}

func Validate(e Extraction) error {
	if !e.ShouldSave && e.Action == "IGNORE" {
		return nil
	}
	if !e.ShouldSave || len(strings.TrimSpace(e.Title)) == 0 || len(e.Title) > 500 || len(e.Content) > 256*1024 || len(e.Reason) == 0 || len(e.SourceMessageIDs) == 0 {
		return errors.New("invalid proposal")
	}
	switch e.Action {
	case "CREATE", "APPEND", "UPDATE", "MERGE", "CONFLICT":
	default:
		return errors.New("invalid action")
	}
	switch e.RiskLevel {
	case "low", "medium", "high":
	default:
		return errors.New("invalid risk")
	}
	switch e.VerificationStatus {
	case "user_provided", "user_confirmed", "ai_suggested", "unverified":
	default:
		return errors.New("invalid verification")
	}
	if e.Action != "CREATE" && e.TargetDocumentID == "" {
		return errors.New("missing target")
	}
	if Sensitive(e.Title + "\n" + e.Summary + "\n" + e.Content + "\n" + e.NewText + "\n" + e.Reason) {
		return errors.New("sensitive proposal")
	}
	return nil
}

// Apply never round-trips an existing rich text document through Markdown.
func Apply(before, contentType string, e Extraction) (string, error) {
	switch e.Action {
	case "CREATE":
		if strings.TrimSpace(e.Content) == "" {
			break
		}
		return e.Content, nil
	case "APPEND":
		if strings.TrimSpace(e.Content) == "" {
			break
		}
		if contentType != "md" {
			return "", errors.New("HTML append requires manual editing")
		}
		if strings.Contains(before, e.Content) {
			return "", errors.New("duplicate append")
		}
		return before + "\n\n" + e.Content, nil
	case "UPDATE":
		if e.OldText == "" || strings.Count(before, e.OldText) != 1 {
			break
		}
		if len(e.OldText) > len(before)/2 {
			return "", errors.New("large edit requires manual editing")
		}
		return strings.Replace(before, e.OldText, e.NewText, 1), nil
	case "MERGE", "CONFLICT":
		return before, nil
	}
	return "", errors.New("cannot apply safely")
}
