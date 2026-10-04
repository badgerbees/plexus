import os

from fastapi import FastAPI
from pydantic import BaseModel
from gliner import GLiNER

app = FastAPI(title="Plexus NER Microservice")

# Default is the PII-specialized GLiNER (GLiNER-large based, 55+ categories).
# PLEXUS_NER_MODEL can point back to urchade/gliner_small-v2.1; the label set
# follows the model choice.
MODEL_ID = os.getenv("PLEXUS_NER_MODEL", "nvidia/gliner-pii")
PII_LABELS = [
    "person name", "first name", "last name",
    "company", "organization",
    "location", "city", "state", "address",
    "email", "phone_number",
    "user_name", "employee_id", "account_number",
]
GENERIC_LABELS = ["person", "organization", "location", "address", "email", "phone number"]

LABELS = PII_LABELS if "pii" in MODEL_ID.lower() else GENERIC_LABELS
THRESHOLD = float(os.getenv("PLEXUS_NER_THRESHOLD", "0.45"))
MAX_MODEL_CHARS = 2000

print(f"Loading GLiNER model {MODEL_ID}...")
model = GLiNER.from_pretrained(MODEL_ID)
try:
    import torch
    if torch.cuda.is_available():
        model = model.to("cuda")
        print("Moved model to CUDA.")
except Exception:
    pass
print("Model loaded. Ready to serve.")


class ExtractRequest(BaseModel):
    text: str
    source_file: str


def map_label(label: str) -> str:
    label = label.lower()
    if label in ("person", "person name", "first name", "last name", "user_name", "username"):
        return "PERSON"
    if label in ("organization", "company"):
        return "ORGANIZATION"
    if label in ("location", "address", "city", "state", "country"):
        return "ADDRESS"
    if label == "email":
        return "EMAIL"
    if label in ("phone number", "phone_number"):
        return "PHONE_NUMBER"
    if label in ("employee id", "employee_id"):
        return "EMPLOYEE_ID"
    if label in ("account number", "account_number"):
        return "ACCOUNT_NUMBER"
    return "PERSON"


def merge_spans(spans):
    """Dedupe overlapping spans, keeping the highest-confidence label.

    The PII model can tag one surface (e.g. a phone number) with several
    labels at once; only the best-scoring annotation should survive.
    """
    ordered = sorted(spans, key=lambda s: -s["confidence"])
    kept = []
    for span in ordered:
        overlaps = any(
            span["start_byte"] < k["end_byte"] and k["start_byte"] < span["end_byte"]
            for k in kept
        )
        if not overlaps:
            kept.append(span)
    kept.sort(key=lambda s: s["start_byte"])
    return kept


@app.post("/extract")
def extract(req: ExtractRequest):
    text = req.text[:MAX_MODEL_CHARS]

    # GLiNER returns character offsets into `text`, a prefix of req.text, so
    # offsets map 1:1 back to the original.
    results = model.predict_entities(text, labels=LABELS, threshold=THRESHOLD)

    spans = []
    for r in results:
        word = r["text"]
        start = r["start"]
        end = r["end"]
        if not word or start >= end:
            continue
        spans.append({
            "raw_text": word,
            "entity_type": map_label(r["label"]),
            "start_byte": start,
            "end_byte": end,
            "confidence": float(r["score"]),
        })

    spans = merge_spans(spans)

    for s in spans:
        s["source_file"] = req.source_file
        ctx_start = max(0, s["start_byte"] - 80)
        ctx_end = min(len(req.text), s["end_byte"] + 80)
        s["context"] = req.text[ctx_start:ctx_end]

    return {"spans": spans}


if __name__ == "__main__":
    import uvicorn
    port = int(os.getenv("PLEXUS_NER_PORT", "5000"))
    uvicorn.run(app, host="127.0.0.1", port=port)
