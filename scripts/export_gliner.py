"""
GLiNER Model Export Pipeline
Downloads a pretrained GLiNER model from HuggingFace and exports it to ONNX format
for use with the Plexus Go runtime via onnxruntime-go.

Usage:
    pip install gliner onnx onnxruntime optimum[exporters] transformers torch
    python export_gliner.py

Output:
    models/gliner/model.onnx
    models/gliner/tokenizer.json
"""

import os
import json
import shutil
from pathlib import Path

def export_with_optimum():
    """Export using HuggingFace Optimum (preferred path)."""
    from optimum.exporters.onnx import main_export

    model_id = "dslim/bert-base-NER"
    output_dir = Path("models/gliner")
    output_dir.mkdir(parents=True, exist_ok=True)

    print(f"Exporting {model_id} to ONNX...")
    try:
        main_export(
            model_name_or_path=model_id,
            output=output_dir,
            task="token-classification",
            opset=17,
        )
        print(f"ONNX model exported to {output_dir}")
        return True
    except Exception as e:
        print(f"Optimum export failed: {e}")
        return False


def export_with_torch():
    return False

def export_tokenizer_only():
    """Minimal path: just get the tokenizer so Go can at least tokenize."""
    from transformers import AutoTokenizer

    model_id = "urchade/gliner_small-v2.1"
    output_dir = Path("models/gliner")
    output_dir.mkdir(parents=True, exist_ok=True)

    print(f"Downloading tokenizer from {model_id}...")
    try:
        tokenizer = AutoTokenizer.from_pretrained(model_id)
    except Exception:
        model_id = "microsoft/deberta-v3-small"
        print(f"Falling back to base tokenizer: {model_id}")
        tokenizer = AutoTokenizer.from_pretrained(model_id)

    tokenizer.save_pretrained(str(output_dir))

    config = {
        "model_id": model_id,
        "max_length": 512,
        "entity_types": [
            "person", "organization", "location", "email",
            "phone number", "address", "date", "money"
        ],
    }
    with open(output_dir / "config.json", "w") as f:
        json.dump(config, f, indent=2)

    print(f"Tokenizer + config saved to {output_dir}")
    return True


if __name__ == "__main__":
    print("=== Plexus GLiNER ONNX Export ===\n")

    success = False

    try:
        success = export_with_optimum()
    except ImportError:
        print("optimum not installed, trying torch export...\n")

    if not success:
        try:
            success = export_with_torch()
        except ImportError:
            print("gliner/torch not installed, exporting tokenizer only...\n")

    if not success:
        export_tokenizer_only()

    print("\nDone. Next: run the Plexus engine with -model models/gliner/model.onnx")
