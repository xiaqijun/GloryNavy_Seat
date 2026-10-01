"""Extract display names from CCP SDE and an installed Tranquility client.

No game code is executed. Pickles accept primitive containers only; globals and
persistent IDs are rejected. Only selected labels, not complete client files,
are committed. See docs/integrations/eve-terminology.zh-CN.md.
"""
import argparse
import configparser
import hashlib
import io
import json
import pickle
from pathlib import Path
import zipfile

ROOT = Path(__file__).resolve().parents[1]


class PrimitiveUnpickler(pickle.Unpickler):
    def find_class(self, module, name):
        raise ValueError(f"Executable pickle global rejected: {module}.{name}")

    def persistent_load(self, pid):
        raise ValueError("Persistent pickle reference rejected")


def digest(data):
    return hashlib.sha256(data).hexdigest()


def pair(names, **source):
    en, zh = names.get("en"), names.get("zh")
    if not isinstance(en, str) or not en:
        raise ValueError(f"Missing English name: {source}")
    if zh is not None and not isinstance(zh, str):
        raise ValueError(f"Invalid Chinese name: {source}")
    if any(c in (en + (zh or "")) for c in ("<", ">", "\n", "\r")):
        raise ValueError(f"Non-plain label: {source}")
    return {"en": en, "zh": zh or "", "source": source}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--client", type=Path, required=True, help="Tranquility tq directory")
    parser.add_argument("--resources", type=Path, required=True, help="Client ResFiles directory")
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    spec = json.loads((ROOT / "scripts/eve-terminology-sources.json").read_text(encoding="utf-8"))
    config = configparser.ConfigParser()
    config.read(args.client / "start.ini")
    if config["main"]["server"] != "Tranquility":
        raise ValueError("Only the Tranquility client is supported")
    client_build = config["main"].getint("build")
    metadata = {}
    with zipfile.ZipFile(args.archive) as archive:
        build = next(json.loads(line)["buildNumber"] for line in archive.read("_sde.jsonl").splitlines() if json.loads(line)["_key"] == "sde")
        if build != client_build:
            raise ValueError("Client and SDE build must match for reproducibility")
        def rows(filename):
            data = archive.read(filename)
            metadata[filename] = digest(data)
            return [json.loads(line) for line in data.splitlines() if line]
        accounting = {r["internalName"]: r for r in rows("accountingEntryTypes.jsonl")}
        roles = {r["shortName"]: r for r in rows("corporationRoles.jsonl")}
        types = {r["_key"]: r for r in rows("types.jsonl")}
    result = {"metadata": {"build": build, "sde_files_sha256": metadata, "client_files": {}}}
    for group, keys, records, filename in [
        ("wallet", spec["wallet"], accounting, "accountingEntryTypes.jsonl"),
        ("roles", spec["roles"], roles, "corporationRoles.jsonl"),
        ("ships", spec["ships"], types, "types.jsonl"),
    ]:
        result[group] = {}
        for key, record_key in keys.items():
            row = records[record_key]
            result[group][key] = pair(row["name"], kind="sde", file=filename, id=row["_key"], key=record_key)
    index = {line.split(",")[0]: line.split(",")[1:] for line in (args.client / "resfileindex.txt").read_text().splitlines()}
    localizations = {}
    for language in ("main", "en-us", "zh"):
        resource = f"res:/localizationfsd/localization_fsd_{language}.pickle"
        record = index[resource]
        root = args.resources.resolve()
        path = (root / record[0]).resolve()
        if not path.is_relative_to(root):
            raise ValueError("Resource path escaped ResFiles")
        data = path.read_bytes()
        if len(data) != int(record[2]) or hashlib.md5(data).hexdigest() != record[1]:
            raise ValueError(f"Client index integrity mismatch: {resource}")
        result["metadata"]["client_files"][resource] = {"sha256": digest(data), "index_md5": record[1]}
        localizations[language] = PrimitiveUnpickler(io.BytesIO(data), encoding="bytes").load()
    labels = {(v[b"FullPath"] + b"/" + v[b"label"]).decode(): v[b"messageID"] for v in localizations["main"][b"labels"].values()}
    for group, keys in spec["client"].items():
        result[group] = {}
        for key, label in keys.items():
            mid = labels[label]
            names = {lang: localizations[file][1][mid][0] for lang, file in (("zh", "zh"), ("en", "en-us"))}
            result[group][key] = pair(names, kind="client", label=label, message_id=mid)
    for group, entries in spec["protocol"].items():
        result.setdefault(group, {}).update({key: pair({"en": name}, kind="protocol", key=key) for key, name in entries.items()})
    output = json.dumps(result, ensure_ascii=False, indent=2) + "\n"
    destination = ROOT / "web/src/lib/eve-terminology.json"
    if args.check:
        if destination.read_text(encoding="utf-8") != output:
            raise ValueError("Generated terminology differs; regenerate and review")
    else:
        destination.write_text(output, encoding="utf-8")
    print(f"{'Verified' if args.check else 'Generated'} build {build}: " + ", ".join(f"{k}={len(v)}" for k, v in result.items() if k != "metadata"))


if __name__ == "__main__":
    main()
