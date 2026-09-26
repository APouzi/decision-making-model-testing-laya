import os
from pathlib import Path

ROOT = Path(__file__).resolve().parent
RUNTIME_DIR = Path(
    os.environ.get(
        "LAYA_RUNTIME_DIR",
        str(Path(os.environ.get("LOCALAPPDATA", str(ROOT))) / "laya-decision-demo"),
    )
)
DEFAULT_MODEL = "typed-decisions"
MODELS_DIR = Path(os.environ.get("LAYA_MODELS_DIR", str(RUNTIME_DIR / "models")))
MODELS = {
    "typed-decisions": {
        "id": "convaiinnovations/laya-typed-decisions",
        "revision": "1a793eb568e6718f15941d08f85432581df534e3",
        "directory": MODELS_DIR / "laya-typed-decisions",
        "label": "Laya Typed-Decisions",
        "context_tokens": 1024,
        "description": "Specialized for customer service, invoices, security incidents, and agent traces.",
    },
    "english": {
        "id": "convaiinnovations/laya",
        "revision": "55cf4c4ebb4ebe31b2550e8bdf3bd21b99753851",
        "directory": MODELS_DIR / "laya",
        "label": "Laya General English",
        "context_tokens": 512,
        "description": "The original English checkpoint, available for comparison.",
    },
}
