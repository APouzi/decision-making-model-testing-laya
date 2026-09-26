package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

//go:embed all:web
var embeddedUI embed.FS

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func jsonResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Write response: %v", err)
	}
}

func problem(w http.ResponseWriter, status int, message string) {
	jsonResponse(w, status, map[string]string{"detail": message})
}

func readRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	if contentType := strings.Split(r.Header.Get("Content-Type"), ";")[0]; contentType != "application/json" {
		problem(w, 415, "Send Content-Type: application/json.")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256*1024)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		problem(w, 413, "Keep requests under 256 KiB.")
		return false
	}
	if err := uniqueJSON(data); err != nil {
		problem(w, 422, "Invalid JSON: "+err.Error())
		return false
	}
	if err := strictJSON(data, target); err != nil {
		problem(w, 422, err.Error())
		return false
	}
	return true
}

func main() {
	port := flag.Int("port", 8765, "HTTP port")
	host := flag.String("host", env("LAYA_HOST", "127.0.0.1"), "HTTP listen address")
	root := flag.String("source-dir", env("LAYA_SOURCE_DIR", "."), "Directory containing inference_worker.py")
	python := flag.String("python", env("LAYA_PYTHON", "python"), "Python executable with Laya installed")
	healthcheck := flag.Bool("healthcheck", false, "Check the running server, then exit")
	download := flag.Bool("download-models", false, "Download the pinned model weights, then exit")
	flag.Parse()
	if *healthcheck {
		client := &http.Client{Timeout: 5 * time.Second}
		response, err := client.Get("http://127.0.0.1:" + strconv.Itoa(*port) + "/api/ready")
		if err != nil {
			log.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			os.Exit(1)
		}
		return
	}
	absoluteRoot, err := filepath.Abs(*root)
	if err != nil {
		log.Fatal(err)
	}
	if *download {
		cmd := exec.Command(*python, "-u", "-B", filepath.Join(absoluteRoot, "download_model.py"))
		cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
		if err := cmd.Run(); err != nil {
			log.Fatal(err)
		}
		return
	}
	ui, err := fs.Sub(embeddedUI, "web")
	if err != nil {
		log.Fatal(err)
	}
	if _, err := fs.Stat(ui, "index.html"); err != nil {
		log.Fatal("The Svelte app is not built. Run build.ps1 or build the Docker image.")
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(*host, strconv.Itoa(*port)))
	if err != nil {
		log.Fatal(err)
	}
	worker, err := startWorker(*python, absoluteRoot)
	if err != nil {
		_ = listener.Close()
		log.Fatal(err)
	}
	defer worker.stop()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("model")
		if key == "" {
			key = defaultModel
		}
		if !modelValid(key) {
			problem(w, 422, "Unknown model.")
			return
		}
		models := worker.snapshot()
		for _, model := range models {
			if model.Key == key {
				jsonResponse(w, 200, struct {
					modelInfo
					App          string      `json:"app"`
					Backend      string      `json:"backend"`
					PID          int         `json:"pid"`
					WorkerPID    int         `json:"worker_pid"`
					DefaultModel string      `json:"default_model"`
					Models       []modelInfo `json:"models"`
				}{model, "laya-decision-demo", "go", os.Getpid(), worker.cmd.Process.Pid, defaultModel, models})
				return
			}
		}
		problem(w, 503, "Model status is unavailable.")
	})
	mux.HandleFunc("GET /api/ready", func(w http.ResponseWriter, r *http.Request) {
		for _, model := range worker.snapshot() {
			if model.Status != "ready" {
				problem(w, 503, "Models are not ready.")
				return
			}
		}
		jsonResponse(w, 200, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("POST /api/predict", func(w http.ResponseWriter, r *http.Request) {
		var request predictionRequest
		if !readRequest(w, r, &request) {
			return
		}
		if err := validatePrediction(&request); err != nil {
			problem(w, 422, err.Error())
			return
		}
		response := worker.predict(r.Context(), request)
		if response.Status != 200 {
			problem(w, response.Status, response.Detail)
			return
		}
		jsonResponse(w, 200, response.Result)
	})
	mux.HandleFunc("POST /api/decide", func(w http.ResponseWriter, r *http.Request) { legacyDecision(w, r, worker) })
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { problem(w, 404, "Unknown API route.") })
	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/docs.html"
		http.FileServerFS(ui).ServeHTTP(w, r)
	})
	files := http.FileServerFS(ui)
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method not allowed", 405)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		// FileServer's directory listing is unnecessary for the playground.
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if info, err := fs.Stat(ui, path); err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
		files.ServeHTTP(w, r)
	}))
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", "no-referrer")
			mux.ServeHTTP(w, r)
		}),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 130 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024,
	}
	stopping, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go func() {
		<-stopping.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			_ = server.Close()
		}
	}()
	log.Printf("Svelte + Go playground listening on http://%s; warming Laya", listener.Addr())
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("HTTP server: %v", err)
	}
}

func legacyDecision(w http.ResponseWriter, r *http.Request, worker *inferenceWorker) {
	var request struct {
		Model    string   `json:"model"`
		Prompt   string   `json:"prompt"`
		Question string   `json:"question"`
		Options  []string `json:"options"`
	}
	if !readRequest(w, r, &request) {
		return
	}
	if !validText(request.Prompt, 6000) || !validText(request.Question, 300) || len(request.Options) < 2 || len(request.Options) > 6 {
		problem(w, 422, "Provide a prompt (1–6000 characters), question (1–300), and 2–6 options.")
		return
	}
	criteria := map[string]string{}
	seen := map[string]bool{}
	for i, label := range request.Options {
		normalized := strings.ToLower(strings.TrimSpace(label))
		if seen[normalized] || !validText(label, 300) {
			problem(w, 422, "Options need distinct wording and 1–300 characters.")
			return
		}
		seen[normalized] = true
		criteria[string(rune('A'+i))] = label
	}
	state, _ := json.Marshal(request.Prompt)
	questions, _ := json.Marshal(map[string]any{"decision": map[string]any{"type": "choice", "instructions": request.Question, "criteria": criteria}})
	native := predictionRequest{Model: request.Model, State: state, Questions: questions}
	if err := validatePrediction(&native); err != nil {
		problem(w, 422, err.Error())
		return
	}
	response := worker.predict(r.Context(), native)
	if response.Status != 200 {
		problem(w, response.Status, response.Detail)
		return
	}
	var result struct {
		Answers map[string]struct {
			Choice        string             `json:"choice"`
			Probabilities map[string]float64 `json:"probabilities"`
		} `json:"answers"`
		ElapsedMS float64 `json:"elapsed_ms"`
		Usage     struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
		Model  string `json:"model"`
		Device string `json:"device"`
	}
	if err := json.NewDecoder(bytes.NewReader(response.Result)).Decode(&result); err != nil {
		problem(w, 502, "Invalid model response.")
		return
	}
	answer := result.Answers["decision"]
	options := make([]map[string]any, 0, len(request.Options))
	for i, label := range request.Options {
		key := string(rune('A' + i))
		options = append(options, map[string]any{"id": key, "label": label, "probability": answer.Probabilities[key]})
	}
	jsonResponse(w, 200, map[string]any{
		"decision": criteria[answer.Choice], "selected": answer.Choice, "probability": answer.Probabilities[answer.Choice],
		"options": options, "elapsed_ms": result.ElapsedMS, "input_tokens": result.Usage.InputTokens, "model": result.Model, "device": result.Device,
	})
}
