"""Build the skill-type reference from an official SDE JSONL archive (no network)."""
import argparse
import json
import math
from pathlib import Path
import zipfile

parser = argparse.ArgumentParser()
parser.add_argument("archive", type=Path)
parser.add_argument("--build", type=int, required=True)
args = parser.parse_args()
if args.build <= 0:
    parser.error("build must be positive")
with zipfile.ZipFile(args.archive) as archive:
    rank_attribute = next(row["_key"] for row in map(json.loads, archive.read("dogmaAttributes.jsonl").splitlines()) if row.get("name") == "skillTimeConstant")
    ranks = {}
    with archive.open("typeDogma.jsonl") as dogma:
        for line in dogma:
            row = json.loads(line)
            for attr in row.get("dogmaAttributes", []):
                if attr["attributeID"] == rank_attribute:
                    ranks[row["_key"]] = attr["value"]
    groups = {
        row["_key"]: row
        for row in map(json.loads, archive.read("groups.jsonl").splitlines())
        if row["categoryID"] == 16
    }
    rows = []
    for row in map(json.loads, archive.read("types.jsonl").splitlines()):
        if row["groupID"] not in groups or not row.get("published"):
            continue
        group = groups[row["groupID"]]
        rank = ranks.get(row["_key"])
        if rank is None or not math.isfinite(rank) or not 0 < rank <= 1000:
            raise ValueError(f"Invalid training multiplier for skill {row['_key']}")
        rows.append({
            "id": str(row["_key"]),
            "name": row["name"].get("zh", row["name"]["en"]),
            "english": row["name"]["en"],
            "group_id": str(row["groupID"]),
            "group": group["name"].get("zh", group["name"]["en"]),
            "group_english": group["name"]["en"],
            "rank": rank,
        })
if not rows:
    raise ValueError("No published skills in the archive")
destination = Path(__file__).resolve().parents[1] / "internal/modules/skills/catalog.json"
destination.write_text(json.dumps({"build": args.build, "skills": sorted(rows, key=lambda r: int(r["id"]))}, ensure_ascii=False, separators=(",", ":")), encoding="utf-8")
print(f"Wrote {len(rows)} skills for build {args.build}")
