# syntax=docker/dockerfile:1

FROM node:22-bookworm-slim AS frontend
WORKDIR /src
COPY frontend/package.json frontend/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run check && npm run build

FROM golang:1.26-alpine AS go-build
WORKDIR /src
COPY backend/ ./
COPY --from=frontend /src/dist/ ./web/
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/laya-demo .

FROM python:3.12-slim-bookworm AS inference-deps
ARG TORCH_BACKEND=cpu
ENV PIP_DISABLE_PIP_VERSION_CHECK=1
RUN python -m venv /opt/venv
COPY requirements.txt /tmp/requirements.txt
RUN --mount=type=cache,target=/root/.cache/pip \
    /opt/venv/bin/pip install torch==2.8.0 --index-url https://download.pytorch.org/whl/${TORCH_BACKEND} \
    && /opt/venv/bin/pip install -r /tmp/requirements.txt

FROM python:3.12-slim-bookworm AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends libgomp1 ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 laya && useradd --uid 10001 --gid laya --no-create-home laya \
    && mkdir -p /data/models /data/cache && chown -R laya:laya /data
WORKDIR /app
COPY --from=inference-deps /opt/venv /opt/venv
COPY --from=go-build /out/laya-demo /app/laya-demo
COPY inference_worker.py model_config.py download_model.py ./
COPY frontend/public/examples.json ./frontend/public/examples.json
ENV PATH="/opt/venv/bin:$PATH" \
    LAYA_HOST=0.0.0.0 LAYA_RUNTIME_DIR=/data \
    PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 \
    HOME=/data HF_HOME=/data/cache/huggingface \
    HF_HUB_DISABLE_TELEMETRY=1 TOKENIZERS_PARALLELISM=false
USER laya
EXPOSE 8765
HEALTHCHECK --interval=20s --timeout=6s --start-period=180s --retries=3 \
    CMD ["/app/laya-demo", "--healthcheck"]
ENTRYPOINT ["/app/laya-demo"]
