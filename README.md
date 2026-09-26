# Laya decision playground

A **Svelte 5** UI and **Go** HTTP backend for Laya's native state + questions format. All 17 examples remain editable. The Go binary embeds the compiled UI and manages a private Python/PyTorch inference worker over stdin/stdout. Python does not expose an HTTP server.

## UI

- `noul`: the winning **Yes** or **No** is the headline and highlighted option. Exactly 50/50 shows **No clear winner**. Custom false/true labels are respected. The raw API value remains P(true).
- `choice`: the selected option and its distribution.
- `score`: the expected position on an ordered, zero-based rubric.
- Both JSON inputs have CodeMirror syntax highlighting, line numbers, folding, live lint errors, duplicate-key detection, and individual **Format JSON** buttons. The question editor also validates Laya question definitions. Invalid input disables Run.
- The expandable API request and response use the same JSON viewer, in read-only mode.

## Run on this Windows machine

Prerequisites: Node 20+, Go 1.25+, Python 3.12, and [uv](https://docs.astral.sh/uv/). Source stays here; dependencies, compiled output, and weights live under `%LOCALAPPDATA%\laya-decision-demo` because Windows Controlled Folder Access protects Documents.

```powershell
.\setup.ps1       # install dependencies, download both pinned models, build UI + Go
.\start.ps1       # http://127.0.0.1:8765; Ctrl+C stops the server and worker
```

After source changes, stop the local Go server and run `./build.ps1`, then `./start.ps1`. Use `./start.ps1 -Port 8766` for another port. Set `LAYA_RUNTIME_DIR` consistently before setup, build, and start to use a different writable directory. `LAYA_MODELS_DIR` can override only the weights directory.

## Docker with Make

Run these commands from the repository root (GNU Make and Docker required):

```sh
make up       # build, start, and wait for both models; CUDA by default
make down     # stop and remove this demo's containers and network
make clean    # also remove this demo's service image; keep all model weights
```

`make up GPU=0` uses the smaller CPU image. `make up PORT=8766` changes the host port. On this Windows machine, Make automatically reuses the existing downloaded weights. Elsewhere, it downloads the pinned models into a persistent Docker volume before starting. Use `MODELS_DIR=/absolute/path/to/models` to reuse another directory, or `MODELS_DIR=` to explicitly use the Docker volume. Keep the same options when running down or clean. These commands never prune other projects or delete model volumes.

## Docker: existing downloaded weights

One container runs Go and the private inference worker. The Dockerfile has separate **Svelte build**, **Go build**, **Python dependency**, and **runtime** stages. The final image contains the stripped Go binary with its embedded UI and the Python inference dependencies; Node, Go, source build dependencies, and package caches stay in build stages. Model weights stay outside the image.

With Docker Desktop using Linux containers and NVIDIA GPU support:

```powershell
.\docker.ps1              # CUDA on the RTX 3080, reuses downloaded weights read-only
# Or choose the smaller image that runs inference on CPU:
.\docker.ps1 -Cpu
```

Stop any other server using 8765 first, or use `-Port 8766`. Both commands build the image and wait for the models to warm. Open **http://127.0.0.1:8765**.

```powershell
docker compose logs -f playground
docker compose down       # stops this demo; keeps downloaded model data
```

## Docker: fresh installation

The default Compose configuration uses CPU PyTorch to keep the image smaller and work without an NVIDIA GPU.

```sh
docker compose build
docker compose run --rm playground --download-models
docker compose up -d --wait --wait-timeout 240
```

The downloader stores both pinned checkpoints in the persistent `models` volume. Downloads happen only when explicitly requested; startup and inference use offline mode. Each model has approximately 804 MiB of weights.

For CUDA, use both files for each command:

```sh
docker compose -f compose.yaml -f compose.gpu.yaml build
docker compose -f compose.yaml -f compose.gpu.yaml run --rm playground --download-models
docker compose -f compose.yaml -f compose.gpu.yaml up -d --wait --wait-timeout 240
```

The CUDA image necessarily includes CUDA libraries and is larger than the CPU image. Neither image contains model weights. To bind an existing weights directory, set `LAYA_MODELS_DIR` to its absolute host path and also add `-f compose.local-models.yaml`. This mount is read-only, so use the host downloader to update its weights. `docker.ps1` sets up these overrides for this machine.

Port 8765 is published only on loopback. Change the host port with `LAYA_PORT`. The container runs as UID 10001 and reports healthy only after both checkpoints have warmed. Only Go listens on a network port.

If Docker reports that port 8765 is already in use, stop the foreground local demo with Ctrl+C before running `make up`, or use `make up PORT=8766`. A native server and the Docker container cannot bind the same port at the same time.

## Models

| Selector | Checkpoint | Context per question |
| --- | --- | --- |
| Laya Typed-Decisions, default | [convaiinnovations/laya-typed-decisions](https://huggingface.co/convaiinnovations/laya-typed-decisions) | 1,024 tokens |
| Laya General English | [convaiinnovations/laya](https://huggingface.co/convaiinnovations/laya) | 512 tokens |

`model_config.py` pins Typed-Decisions to `1a793eb568e6718f15941d08f85432581df534e3` and the original to `55cf4c4ebb4ebe31b2550e8bdf3bd21b99753851`. Both load once and warm with the refund example. Inference explicitly uses FP32. Local startup selects CUDA when available; set `LAYA_DEVICE=cpu` for CPU.

Typed-Decisions specializes in customer service, invoices, security incidents, and agent traces. It is not uniformly better on every example. Compare the same JSON using the model selector. Decisions are displayed, never executed.

## Examples and source

This folder is one Git working tree for frontend, backend, and container configuration, initialized on `main`. On this machine, Git metadata lives at `%LOCALAPPDATA%\laya-decision-demo\git\decision_making_demo.git`, linked by the root `.git` file, because Windows blocks Git writes under Documents. Normal Git commands work from this project root. Keep that metadata directory when moving or backing up this working copy; model cleanup does not touch it.

All **17 examples / 56 questions** are in `frontend/public/examples.json`, including the exact refund demo and temperature balance. Titles and hints are UI guidance; only the state and question definitions go to the model.

- `frontend/src/App.svelte`: example picker, model selection, request lifecycle.
- `frontend/src/DecisionCard.svelte`: native typed answers and winner highlighting.
- `frontend/src/JsonEditor.svelte`: JSON input and read-only viewers.
- `frontend/src/lib/json.js`: syntax, duplicate-key, and question validation.
- `backend/`: Go server, request validation, embedded UI, worker lifecycle.
- `inference_worker.py`: offline model loading, warmup, token budget checks, predictions.
- `Dockerfile`, `compose*.yaml`: multi-stage image and CPU/GPU/weights configuration.

To develop the UI, run `npm ci` and `npm run dev` from `frontend`; Vite proxies `/api` to Go on 8765. If Windows blocks writes in Documents, use the copied frontend at `%LOCALAPPDATA%\laya-decision-demo\build\frontend` or run `build.ps1`. Production always serves the compiled Svelte UI from Go.

## API

Local documentation: **http://127.0.0.1:8765/docs**.

- `GET /api/health`: both model statuses, revisions, device and precision, plus Go and worker process IDs; accepts `?model=english`.
- `GET /api/ready`: 200 when both models are warm; otherwise 503.
- `POST /api/predict`: `{ "model": "typed-decisions", "state": {...}, "questions": {...} }`.
- `POST /api/decide`: retained compatibility for the original `prompt`, `question`, and `options` shape.

Use `Content-Type: application/json`. There are 1–8 questions per request and 2–8 options or rubric levels. Noul supports optional `criteria` and `labels` using false/true keys. Unknown fields and duplicate keys are rejected. JSON option order is preserved when Go passes requests to Python.

Invalid definitions or input exceeding the actual tokenizer budget return 422; payloads over 256 KiB return 413; a busy worker returns 429; unavailable models return 503. The Go server bounds inference to one request at a time and two minutes. Disconnecting a browser does not allow another request to consume its answer. Go shuts down its worker when the server stops.

The response preserves native `answers` and `usage`, plus model ID, revision, device, precision, timing, and question count. `usage.input_tokens` sums every question and its repeated state, so it can exceed the individual context window. The SDK's action metadata is shown only in raw JSON and is never used to execute an action.

Build checks and real API/browser smoke checks are used for this demo. No test suite is required.
