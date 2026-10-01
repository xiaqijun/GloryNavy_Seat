"""Build fitting UI group labels from an official JSONL SDE archive."""
import argparse
import json
import pathlib
import zipfile

parser = argparse.ArgumentParser()
parser.add_argument("archive", type=pathlib.Path)
parser.add_argument("--build", required=True, type=int)
args = parser.parse_args()
with zipfile.ZipFile(args.archive) as archive:
    rows = [json.loads(line) for line in archive.read("groups.jsonl").splitlines() if line]
groups = {str(row["_key"]): row["name"].get("zh") or row["name"].get("en")
          for row in rows if row.get("categoryID") in (6, 7, 8, 18, 20, 32, 65, 66, 87)}
output = pathlib.Path(__file__).resolve().parents[1] / "web/src/modules/fittings/group-names.json"
output.write_text(json.dumps({"build": args.build, "groups": groups}, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(f"Generated {len(groups)} group labels for SDE {args.build}")
