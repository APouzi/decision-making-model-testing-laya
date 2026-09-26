# One repository and one container for the Svelte UI, Go API, and Laya worker.
# Use `make up GPU=0` for the smaller CPU image; `PORT=8766` changes the host port.
GPU ?= 1
PORT ?= 8765

# Reuse the models already downloaded on this Windows machine. On a fresh
# machine, an empty MODELS_DIR uses a persistent Docker volume instead.
DEFAULT_MODELS_DIR := $(subst \,/,$(LOCALAPPDATA))/laya-decision-demo/models
ifeq ($(origin MODELS_DIR),undefined)
MODELS_DIR := $(LAYA_MODELS_DIR)
ifeq ($(strip $(MODELS_DIR)),)
ifneq ($(wildcard $(DEFAULT_MODELS_DIR)/laya/model.safetensors),)
ifneq ($(wildcard $(DEFAULT_MODELS_DIR)/laya-typed-decisions/model.safetensors),)
MODELS_DIR := $(DEFAULT_MODELS_DIR)
endif
endif
endif
endif

export LAYA_PORT := $(PORT)
COMPOSE := docker compose -f compose.yaml
ifeq ($(GPU),1)
COMPOSE += -f compose.gpu.yaml
endif
ifneq ($(strip $(MODELS_DIR)),)
export LAYA_MODELS_DIR := $(MODELS_DIR)
COMPOSE += -f compose.local-models.yaml
endif

.PHONY: up down clean
.DEFAULT_GOAL := up

up:
	$(COMPOSE) build
ifeq ($(strip $(MODELS_DIR)),)
	$(COMPOSE) run --rm playground --download-models
endif
	$(COMPOSE) up -d --wait --wait-timeout 240

down:
	$(COMPOSE) down --remove-orphans

# Only this Compose project's containers, network, and service image are removed.
# Model volumes, bind-mounted weights, source files, and other projects are kept.
clean:
	$(COMPOSE) down --remove-orphans --rmi all
