package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type modelInfo struct {
	Key           string  `json:"key"`
	Model         string  `json:"model"`
	Label         string  `json:"label"`
	Revision      string  `json:"revision"`
	Description   string  `json:"description"`
	ContextTokens int     `json:"context_tokens"`
	Device        *string `json:"device"`
	DeviceName    string  `json:"device_name"`
	Precision     string  `json:"precision"`
	Status        string  `json:"status"`
	Error         *string `json:"error"`
}

type workerResponse struct {
	ID     uint64          `json:"id"`
	Event  string          `json:"event"`
	Models []modelInfo     `json:"models"`
	Status int             `json:"status"`
	Result json.RawMessage `json:"result"`
	Detail string          `json:"detail"`
}

type inferenceWorker struct {
	cmd       *exec.Cmd
	input     io.WriteCloser
	responses chan workerResponse
	done      chan struct{}
	busy      chan struct{}
	mu        sync.RWMutex
	models    []modelInfo
	sequence  atomic.Uint64
}

func startWorker(python, root string) (*inferenceWorker, error) {
	cmd := exec.Command(python, "-u", "-B", filepath.Join(root, "inference_worker.py"))
	prepareProcess(cmd)
	cmd.Dir = root
	workerEnv := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(key, "TYPESAFE_API_KEY") {
			continue
		}
		workerEnv = append(workerEnv, entry)
	}
	cmd.Env = append(workerEnv, "PYTHONIOENCODING=utf-8", "PYTHONDONTWRITEBYTECODE=1")
	cmd.Stderr = os.Stderr
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		_ = input.Close()
		return nil, err
	}
	w := &inferenceWorker{cmd: cmd, input: input, responses: make(chan workerResponse, 1), done: make(chan struct{}), busy: make(chan struct{}, 1), models: []modelInfo{
		{Key: "typed-decisions", Model: "convaiinnovations/laya-typed-decisions", Label: "Laya Typed-Decisions", Status: "loading", ContextTokens: 1024},
		{Key: "english", Model: "convaiinnovations/laya", Label: "Laya General English", Status: "loading", ContextTokens: 512},
	}}
	if err := cmd.Start(); err != nil {
		_ = input.Close()
		_ = output.Close()
		return nil, err
	}
	go func() {
		scanner := bufio.NewScanner(output)
		scanner.Buffer(make([]byte, 4096), 4*1024*1024)
		for scanner.Scan() {
			var message workerResponse
			if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
				log.Printf("Invalid inference worker response: %v", err)
				_ = terminateProcess(cmd)
				break
			}
			if message.Event == "models" {
				w.mu.Lock()
				w.models = message.Models
				w.mu.Unlock()
			} else {
				select {
				case w.responses <- message:
				default:
					log.Print("Unexpected inference worker response")
					_ = terminateProcess(cmd)
				}
			}
		}
		if err := scanner.Err(); err != nil {
			log.Printf("Inference worker pipe: %v", err)
			_ = terminateProcess(cmd)
		}
		err := cmd.Wait()
		message := "Inference worker stopped. Restart the server."
		if err != nil {
			log.Printf("%s %v", message, err)
		}
		w.mu.Lock()
		for i := range w.models {
			w.models[i].Status = "error"
			w.models[i].Error = &message
		}
		w.mu.Unlock()
		close(w.done)
	}()
	return w, nil
}

func (w *inferenceWorker) snapshot() []modelInfo {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return append([]modelInfo(nil), w.models...)
}

func (w *inferenceWorker) predict(ctx context.Context, request predictionRequest) workerResponse {
	for _, model := range w.snapshot() {
		if model.Key == request.Model && model.Status != "ready" {
			detail := "This model is still loading and warming. Try again shortly."
			if model.Error != nil {
				detail = *model.Error
			}
			return workerResponse{Status: 503, Detail: detail}
		}
	}
	select {
	case w.busy <- struct{}{}:
	default:
		return workerResponse{Status: 429, Detail: "A model is already running. Try again in a moment."}
	}
	finished := make(chan workerResponse, 1)
	go func() {
		defer func() { <-w.busy }()
		id := w.sequence.Add(1)
		err := json.NewEncoder(w.input).Encode(struct {
			ID      uint64            `json:"id"`
			Request predictionRequest `json:"request"`
		}{id, request})
		if err != nil {
			finished <- workerResponse{Status: 503, Detail: "Inference worker is unavailable. Restart the server."}
			return
		}
		timer := time.NewTimer(120 * time.Second)
		defer timer.Stop()
		select {
		case response := <-w.responses:
			if response.ID != id || response.Status < 200 || response.Status > 599 {
				_ = terminateProcess(w.cmd)
				response = workerResponse{Status: 502, Detail: "Invalid response from inference worker. Restart the server."}
			}
			finished <- response
		case <-w.done:
			finished <- workerResponse{Status: 503, Detail: "Inference worker stopped. Restart the server."}
		case <-timer.C:
			_ = terminateProcess(w.cmd)
			finished <- workerResponse{Status: 504, Detail: "Inference exceeded two minutes. Restart the server to reload the models."}
		}
	}()
	// If a browser disconnects, keep the inference slot until its answer arrives.
	// A subsequent request can never consume the previous caller's answer.
	select {
	case response := <-finished:
		return response
	case <-ctx.Done():
		return workerResponse{Status: 408, Detail: "Request canceled."}
	}
}

func (w *inferenceWorker) stop() {
	_ = w.input.Close()
	select {
	case <-w.done:
	case <-time.After(5 * time.Second):
		if err := terminateProcess(w.cmd); err != nil {
			log.Print(fmt.Errorf("stop inference worker: %w", err))
		}
		<-w.done
	}
}
