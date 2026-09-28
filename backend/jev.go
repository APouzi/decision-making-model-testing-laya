package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const jevEndpoint = "https://api.typesafe.ai/v1/systemone"

var validJevModel = regexp.MustCompile(`^[A-Za-z0-9._-]{1,80}$`)

type jevRequest struct {
	Model     string          `json:"model"`
	State     json.RawMessage `json:"state"`
	Questions json.RawMessage `json:"questions"`
}

func jevHealthHandler(model, apiKey string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, map[string]any{
			"provider":   "TypeSafe",
			"model":      model,
			"configured": strings.TrimSpace(apiKey) != "",
		})
	}
}

func jevPredictionHandler(model, apiKey string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(apiKey) == "" {
			problem(w, http.StatusServiceUnavailable, "Jev is not configured. Add TYPESAFE_API_KEY to .env and restart the server.")
			return
		}
		var request jevRequest
		if !readRequest(w, r, &request) {
			return
		}
		if request.Model != "" && request.Model != model {
			problem(w, http.StatusUnprocessableEntity, "The model must match the model configured by the server.")
			return
		}
		if !validJevModel.MatchString(model) {
			problem(w, http.StatusInternalServerError, "JEV_MODEL must be a valid model identifier.")
			return
		}
		validated := predictionRequest{Model: defaultModel, State: request.State, Questions: request.Questions}
		if err := validatePrediction(&validated); err != nil {
			problem(w, http.StatusUnprocessableEntity, err.Error())
			return
		}

		payload, err := json.Marshal(jevRequest{Model: model, State: request.State, Questions: request.Questions})
		if err != nil {
			problem(w, http.StatusInternalServerError, "Could not encode the Jev request.")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
		defer cancel()
		upstreamRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, jevEndpoint, bytes.NewReader(payload))
		if err != nil {
			problem(w, http.StatusInternalServerError, "Could not create the Jev request.")
			return
		}
		upstreamRequest.Header.Set("Authorization", "Bearer "+apiKey)
		upstreamRequest.Header.Set("Content-Type", "application/json")

		started := time.Now()
		client := &http.Client{
			Timeout: 120 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		response, err := client.Do(upstreamRequest)
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
				problem(w, http.StatusGatewayTimeout, "Jev did not respond within two minutes.")
				return
			}
			problem(w, http.StatusBadGateway, "Could not reach the Jev API. Check the server's network connection.")
			return
		}
		defer response.Body.Close()
		body, err := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024))
		if err != nil {
			problem(w, http.StatusBadGateway, "Could not read the Jev API response.")
			return
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			status := response.StatusCode
			if status < 400 || status > 599 {
				status = http.StatusBadGateway
			}
			problem(w, status, jevErrorDetail(body, response.StatusCode))
			return
		}
		var result map[string]json.RawMessage
		if err := json.Unmarshal(body, &result); err != nil || len(result) == 0 {
			problem(w, http.StatusBadGateway, "Jev returned an invalid JSON response.")
			return
		}
		for _, field := range []string{"model", "answers", "usage"} {
			if len(result[field]) == 0 || bytes.Equal(result[field], []byte("null")) {
				problem(w, http.StatusBadGateway, "Jev returned an incomplete response.")
				return
			}
		}
		elapsed, _ := json.Marshal(float64(time.Since(started).Microseconds()) / 1000)
		result["elapsed_ms"] = elapsed
		jsonResponse(w, http.StatusOK, result)
	}
}

func jevErrorDetail(body []byte, status int) string {
	var upstream struct {
		Detail  string `json:"detail"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &upstream) == nil {
		for _, message := range []string{upstream.Detail, upstream.Error, upstream.Message} {
			message = strings.TrimSpace(message)
			if message != "" && len(message) <= 500 {
				return message
			}
		}
	}
	switch status {
	case http.StatusUnauthorized:
		return "Jev rejected the API key. Check TYPESAFE_API_KEY in .env."
	case http.StatusTooManyRequests:
		return "Jev rate limit reached. Wait briefly and try again."
	case 529:
		return "Jev is temporarily overloaded. Wait briefly and try again."
	default:
		return fmt.Sprintf("Jev API request failed with status %d.", status)
	}
}
