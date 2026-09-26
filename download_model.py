"""Download the two pinned checkpoints used by the demo, without remote code."""

import os

from model_config import MODELS, RUNTIME_DIR

os.environ.setdefault("HF_HOME", str(RUNTIME_DIR / "cache" / "huggingface"))
os.environ.setdefault("HF_HUB_DISABLE_TELEMETRY", "1")

from huggingface_hub import snapshot_download


def main():
    for model in MODELS.values():
        print(f"Downloading {model['id']} @ {model['revision']}", flush=True)
        snapshot_download(
            repo_id=model["id"],
            revision=model["revision"],
            local_dir=model["directory"],
            allow_patterns=[
                "README.md", "LICENSE*", "rl_agent_config.json", "model.safetensors",
                "encoder/*.json", "tokenizer/*",
            ],
            max_workers=4,
        )
        size = (model["directory"] / "model.safetensors").stat().st_size
        print(f"Ready: {model['directory']} ({size / 1024**2:.1f} MiB of weights)", flush=True)


if __name__ == "__main__":
    main()
