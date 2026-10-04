"""
Export a sampled slice of TonicAI/Privacy-Bench (ground_truth_spans) to a
plain JSONL file for the Go benchmark harness.

Usage:
    python scripts/export_privacybench.py [num_docs]

Output:
    testdata/privacybench.jsonl   (one JSON object per line:
        {"id": "...", "text": "...",
         "spans": [{"label": "NAME_GIVEN", "start": 0, "end": 4}, ...]})

Span offsets are Python character offsets into `text`, matching the offsets
the Plexus NER microservice returns.
"""

import json
import random
import sys
from pathlib import Path

from huggingface_hub import hf_hub_download

TASKS = [
    "aaron_pfizer", "camille_nike", "daniel_lincoln_financial_group",
    "devon_marriott_international", "elena_stripe", "grace_deere_company",
    "hannah_salesforce", "isabella_chipotle", "malik_spotify",
    "marcus_boeing", "megan_donovan_eli_lilly", "morgan_morgan_stanley",
    "nora_caterpillar", "priya_charles_schwab", "ray_raymond_james",
    "renee_tyson_foods", "sofia_netflix", "tanya_kroger",
    "victor_delta_air_lines", "wells_wells_fargo", "yusuf_procter_gamble",
]


def main() -> None:
    limit = int(sys.argv[1]) if len(sys.argv) > 1 else 1000

    all_rows = []
    for task in TASKS:
        path = hf_hub_download(
            "TonicAI/Privacy-Bench",
            f"ground_truth/{task}/spans.jsonl",
            repo_type="dataset",
        )
        with open(path, encoding="utf-8") as f:
            for line in f:
                rec = json.loads(line)
                text = rec.get("text") or ""
                if not text.strip():
                    continue
                spans = [
                    {"label": s["label"], "start": s["start"], "end": s["end"]}
                    for s in rec.get("ground_truth_spans", [])
                    if s["start"] >= 0 and s["end"] <= len(text) and s["start"] < s["end"]
                ]
                all_rows.append({
                    "id": f"{task}:{rec['meta'].get('row_id', '?')}",
                    "text": text,
                    "spans": spans,
                })

    rng = random.Random(42)
    sample = rng.sample(all_rows, min(limit, len(all_rows)))

    out_dir = Path("testdata")
    out_dir.mkdir(parents=True, exist_ok=True)
    out_path = out_dir / "privacybench.jsonl"
    with open(out_path, "w", encoding="utf-8") as f:
        for row in sample:
            f.write(json.dumps(row, ensure_ascii=False) + "\n")

    from collections import Counter
    label_counts = Counter()
    span_total = 0
    for row in sample:
        for s in row["spans"]:
            label_counts[s["label"]] += 1
            span_total += 1

    print(f"wrote {len(sample)} docs / {span_total} spans to {out_path}")
    for label, count in label_counts.most_common():
        print(f"  {label}: {count}")


if __name__ == "__main__":
    main()
