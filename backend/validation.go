package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const defaultModel = "typed-decisions"

// RawMessage is intentional: Laya can be sensitive to option order. Validation
// must not reorder state, questions, or criteria before sending them to Python.
type predictionRequest struct {
	Model     string          `json:"model"`
	State     json.RawMessage `json:"state"`
	Questions json.RawMessage `json:"questions"`
}

type question struct {
	Type         string          `json:"type"`
	Instructions string          `json:"instructions"`
	Criteria     json.RawMessage `json:"criteria,omitempty"`
	Labels       json.RawMessage `json:"labels,omitempty"`
}

func strictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("provide exactly one JSON value")
	}
	return nil
}

// Reject duplicate keys rather than letting an editor and the model disagree
// about which value was supplied. This also applies to direct API clients.
func uniqueJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 64 {
			return fmt.Errorf("JSON nesting exceeds 64 levels")
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch token {
		case json.Delim('{'):
			seen := make(map[string]bool)
			for decoder.More() {
				token, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := token.(string)
				if !ok {
					return fmt.Errorf("object keys must be strings")
				}
				if seen[key] {
					return fmt.Errorf("duplicate JSON key %q", key)
				}
				seen[key] = true
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
		case json.Delim('['):
			for decoder.More() {
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
		}
		return err
	}
	if err := walk(0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("provide exactly one JSON value")
	}
	return nil
}

func validText(value string, max int) bool {
	return strings.TrimSpace(value) != "" && utf8.RuneCountInString(value) <= max
}

func modelValid(key string) bool { return key == "typed-decisions" || key == "english" }
func omitted(value json.RawMessage) bool {
	return len(value) == 0 || bytes.Equal(value, []byte("null"))
}

func validatePrediction(request *predictionRequest) error {
	if request.Model == "" {
		request.Model = defaultModel
	}
	if !modelValid(request.Model) {
		return fmt.Errorf("model must be typed-decisions or english")
	}
	var state any
	if err := json.Unmarshal(request.State, &state); err != nil {
		return fmt.Errorf("state: provide valid JSON")
	}
	nonempty := false
	switch value := state.(type) {
	case string:
		nonempty = strings.TrimSpace(value) != ""
	case []any:
		nonempty = len(value) > 0
	case map[string]any:
		nonempty = len(value) > 0
	default:
		return fmt.Errorf("state must be an object, array, or string")
	}
	if !nonempty {
		return fmt.Errorf("state: provide a non-empty value")
	}
	compact := new(bytes.Buffer)
	_ = json.Compact(compact, request.State)
	if utf8.RuneCount(compact.Bytes()) > 20000 {
		return fmt.Errorf("state: keep it under 20,000 characters")
	}
	var questions map[string]json.RawMessage
	if err := json.Unmarshal(request.Questions, &questions); err != nil || len(questions) < 1 || len(questions) > 8 {
		return fmt.Errorf("questions must be an object with 1 to 8 named questions")
	}
	for name, raw := range questions {
		if !validText(name, 80) {
			return fmt.Errorf("question names need 1–80 characters")
		}
		var q question
		if err := strictJSON(raw, &q); err != nil {
			return fmt.Errorf("questions.%s: %w", name, err)
		}
		if err := validateQuestion(q); err != nil {
			return fmt.Errorf("questions.%s: %w", name, err)
		}
	}
	return nil
}

func validateQuestion(q question) error {
	if !validText(q.Instructions, 600) {
		return fmt.Errorf("instructions need 1–600 characters")
	}
	switch q.Type {
	case "choice", "score":
		if !omitted(q.Labels) {
			return fmt.Errorf("labels are only supported for noul")
		}
		var options []string
		if err := json.Unmarshal(q.Criteria, &options); err != nil || options == nil {
			if q.Type == "score" {
				return fmt.Errorf("score requires criteria as an ordered list")
			}
			var criteria map[string]string
			if err := json.Unmarshal(q.Criteria, &criteria); err != nil || criteria == nil {
				return fmt.Errorf("choice requires criteria as an object or list")
			}
			for key, value := range criteria {
				if !validText(key, 80) {
					return fmt.Errorf("option names need 1–80 characters")
				}
				options = append(options, value)
			}
		} else {
			seen := map[string]bool{}
			for _, value := range options {
				trimmed := strings.TrimSpace(value)
				if seen[trimmed] {
					return fmt.Errorf("give every criterion distinct wording")
				}
				seen[trimmed] = true
			}
		}
		if len(options) < 2 || len(options) > 8 {
			return fmt.Errorf("use 2 to 8 criteria")
		}
		for _, value := range options {
			if !validText(value, 300) {
				return fmt.Errorf("each criterion needs 1–300 characters")
			}
		}
	case "noul":
		if !omitted(q.Criteria) {
			var criteria map[string]string
			if err := json.Unmarshal(q.Criteria, &criteria); err != nil {
				return fmt.Errorf("noul criteria must use false/true keys with text descriptions")
			}
			for key, value := range criteria {
				if (key != "false" && key != "true") || !validText(value, 300) {
					return fmt.Errorf("noul criteria must use false/true keys with text descriptions")
				}
			}
		}
		if !omitted(q.Labels) {
			var labels map[string]string
			if err := json.Unmarshal(q.Labels, &labels); err != nil || len(labels) != 2 || !validText(labels["false"], 300) || !validText(labels["true"], 300) || strings.TrimSpace(labels["false"]) == strings.TrimSpace(labels["true"]) {
				return fmt.Errorf("labels must map false and true to different text")
			}
		}
	default:
		return fmt.Errorf("type must be noul, choice, or score")
	}
	return nil
}
