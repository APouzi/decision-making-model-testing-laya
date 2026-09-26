"""Private JSON-lines inference worker. The Go server owns the public HTTP API."""

import contextlib
import json
import logging
import os
import sys
from time import perf_counter

from model_config import DEFAULT_MODEL, MODELS, ROOT, RUNTIME_DIR

os.environ.update(HF_HUB_OFFLINE="1", TRANSFORMERS_OFFLINE="1", USE_TF="0", USE_FLAX="0")
os.environ.setdefault("HF_HOME", str(RUNTIME_DIR / "cache" / "huggingface"))
os.environ.setdefault("HF_HUB_DISABLE_TELEMETRY", "1")
os.environ.setdefault("TOKENIZERS_PARALLELISM", "false")
logging.basicConfig(stream=sys.stderr, level=logging.INFO, format="%(levelname)s %(message)s")
logger = logging.getLogger("laya.worker")
wire = sys.stdout
agents = {}
statuses = {key: {"status": "loading", "error": None} for key in MODELS}


def emit(message):
    wire.write(json.dumps(message, ensure_ascii=False, allow_nan=False) + "\n")
    wire.flush()


def publish_models():
    models = []
    for key, spec in MODELS.items():
        agent = agents.get(key)
        models.append({
            "key": key, "model": spec["id"], "label": spec["label"],
            "revision": spec["revision"], "description": spec["description"],
            "context_tokens": spec["context_tokens"], "precision": "float32",
            "device": str(agent.device) if agent is not None else None,
            "device_name": statuses[key].get("device_name", ""), **statuses[key],
        })
    emit({"event": "models", "default_model": DEFAULT_MODEL, "models": models})


def load_models():
    publish_models()
    try:
        import laya
        import torch

        torch.set_num_threads(min(8, os.cpu_count() or 4))
        sample = json.loads((ROOT / "frontend" / "public" / "examples.json").read_text(encoding="utf-8"))["refund"]
    except Exception as exc:
        for key in MODELS:
            statuses[key] = {"status": "error", "error": str(exc)}
        logger.exception("Could not initialize Laya")
        publish_models()
        return

    for key, spec in MODELS.items():
        try:
            if not (spec["directory"] / "model.safetensors").is_file():
                raise FileNotFoundError(f"Missing {spec['label']} weights. Download the models first; see README.md.")
            agent = laya.load(str(spec["directory"]), device=os.environ.get("LAYA_DEVICE") or None)
            # laya 0.3.20 ignores LAYA_CUDA_AMP. Preserve the demo's explicit FP32.
            agent.amp_enabled = False
            agent.dtype = torch.float32
            agent.predict(sample["state"], sample["questions"])
            agents[key] = agent
            device_name = torch.cuda.get_device_name(agent.device) if agent.device.type == "cuda" else str(agent.device).upper()
            statuses[key] = {"status": "ready", "error": None, "device_name": device_name}
            logger.info("%s loaded and warmed on %s (FP32)", spec["id"], device_name)
        except Exception as exc:
            statuses[key] = {"status": "error", "error": str(exc)}
            logger.exception("Could not load %s", spec["id"])
        publish_models()


def check_token_budget(agent, state, questions):
    from laya.common import render_options, serialize_state

    def count(text):
        return len(agent.tok(text.replace(agent.tok.mask_token, " "), add_special_tokens=False)["input_ids"])

    max_len = agent.cfg.get("max_len", 512)
    head_budget = agent.cfg.get("head_max_len", 192)
    state_tokens = count(serialize_state(state))
    for name, question in questions.items():
        criteria = question.get("criteria")
        if question["type"] == "choice" and isinstance(criteria, list):
            criteria = {label: None for label in criteria}
        internal = {"t": question["type"], "ins": question["instructions"], "crit": criteria}
        if question.get("labels") is not None:
            internal["labels"] = question["labels"]
        lengths = [count(" " + option) for option in render_options(internal)]
        option_tokens = sum(length + 1 for length in lengths)
        question_tokens = count(f"{question['type']} question: {question['instructions']}")
        if max(lengths) > 48 or option_tokens > head_budget - 16 or question_tokens > head_budget - option_tokens:
            raise ValueError(f"Question '{name}' exceeds the question/option token budget. Shorten its instructions or criteria.")
        available = max_len - question_tokens - option_tokens - 4
        if state_tokens > available:
            raise ValueError(f"State uses {state_tokens} tokens; {available} fit with question '{name}'. Shorten the state or question.")


def predict(request):
    key = request["model"]
    if statuses[key]["status"] != "ready":
        return {"status": 503, "detail": statuses[key]["error"] or "This model is still loading and warming."}
    agent = agents[key]
    # Preserve question, state, and criterion order from the original HTTP JSON.
    questions = {name: {k: v for k, v in q.items() if v is not None} for name, q in request["questions"].items()}
    check_token_budget(agent, request["state"], questions)
    started = perf_counter()
    result = agent.predict(request["state"], questions)
    return {"status": 200, "result": {
        **result, "model": MODELS[key]["id"], "model_key": key,
        "revision": MODELS[key]["revision"], "device": str(agent.device),
        "precision": "float32", "elapsed_ms": round((perf_counter() - started) * 1000, 1),
        "question_count": len(questions),
    }}


def main():
    # Library progress messages must never contaminate the JSON protocol.
    with contextlib.redirect_stdout(sys.stderr):
        load_models()
        for line in sys.stdin:
            request_id = None
            try:
                message = json.loads(line)
                request_id = message["id"]
                response = predict(message["request"])
            except ValueError as exc:
                response = {"status": 422, "detail": str(exc)}
            except Exception:
                logger.exception("Inference failed")
                response = {"status": 500, "detail": "The model could not finish these questions. Check the server log."}
            emit({"id": request_id, **response})


if __name__ == "__main__":
    main()
