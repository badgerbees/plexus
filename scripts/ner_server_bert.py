from fastapi import FastAPI
from pydantic import BaseModel
from transformers import pipeline

app = FastAPI(title="Plexus NER Microservice")

# We use the standard BERT NER model that works perfectly for our use case.
# "max" aggregation merges consecutive tokens of the same entity group into a
# single span ("Alex Rivera" instead of "Alex" + "Rivera"), which the Go side
# relies on for alias linkage.
print("Loading NER model into memory...")
ner = pipeline("ner", model="dslim/bert-base-NER", aggregation_strategy="max")
print("Model loaded. Ready to serve.")

MAX_MODEL_CHARS = 3500


class ExtractRequest(BaseModel):
    text: str
    source_file: str


def map_entity_group(group: str) -> str:
    group = group.upper()
    if group == "PER": return "PERSON"
    if group == "ORG": return "ORGANIZATION"
    if group == "LOC": return "ADDRESS"
    return "PERSON"


def titlecase_words(text: str) -> str:
    """Upper-case the first letter of every whitespace-separated word.

    Length-preserving per code point, so character offsets of the original
    text stay valid on the transformed copy.
    """
    out = []
    up = True
    for ch in text:
        if ch.isalpha():
            out.append(ch.upper() if up else ch)
            up = False
        else:
            out.append(ch)
            if ch.isspace():
                up = True
    return "".join(out)


def run_ner(text: str, min_score: float = 0.0):
    if len(text) > MAX_MODEL_CHARS:
        text = text[:MAX_MODEL_CHARS]
    spans = []
    for r in ner(text):
        if float(r["score"]) < min_score:
            continue
        word = r["word"]
        start = r["start"]
        end = r["end"]

        # Merged spans can carry leading/trailing whitespace; trim it so the
        # byte offsets in the response point at the entity itself.
        lead = len(word) - len(word.lstrip())
        trail = len(word) - len(word.rstrip())
        word = word.strip()
        start += lead
        end -= trail
        if not word:
            continue
        spans.append({
            "raw_text": word,
            "entity_type": map_entity_group(r["entity_group"]),
            "start_byte": start,
            "end_byte": end,
            "confidence": float(r["score"]),
        })
    return spans


def merge_spans(a, b):
    """Union of two span lists, dropping duplicates that overlap a kept span."""
    merged = list(a)
    for span in b:
        overlaps = any(
            span["start_byte"] < k["end_byte"] and k["start_byte"] < span["end_byte"]
            for k in merged
        )
        if not overlaps:
            merged.append(span)
    merged.sort(key=lambda s: s["start_byte"])
    return merged


@app.post("/extract")
def extract(req: ExtractRequest):
    spans = run_ner(req.text)

    # Casual chat is frequently all-lowercase, which cased BERT largely
    # misses. Run a second pass on a title-cased copy of the text; the
    # transform is length-preserving, so offsets map back 1:1. The augmented
    # pass requires higher confidence to suppress case-injection noise.
    augmented = titlecase_words(req.text)
    if augmented != req.text:
        spans = merge_spans(spans, run_ner(augmented, min_score=0.75))

    for s in spans:
        s["source_file"] = req.source_file
        ctx_start = max(0, s["start_byte"] - 80)
        ctx_end = min(len(req.text), s["end_byte"] + 80)
        s["context"] = req.text[ctx_start:ctx_end]

    return {"spans": spans}


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="127.0.0.1", port=5000)
