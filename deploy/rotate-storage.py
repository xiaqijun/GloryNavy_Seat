#!/usr/bin/env python3
"""Rotate only project release directories and paired PostgreSQL backups.

Dry run by default. Application data, Docker volumes, logs and temporary files
are outside this tool's deletion scope. See docs/storage-retention.md.
"""
import argparse
import datetime as dt
import fcntl
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import time

VERSION = re.compile(r"v\d+\.\d+\.\d+(?:-[A-Za-z0-9.-]+)?\Z")


def regular(path):
    return path.exists() and stat.S_ISREG(path.lstat().st_mode)


def fingerprint(path):
    s = path.lstat()
    return [s.st_dev, s.st_ino, s.st_size, s.st_mtime_ns]


def release_info(path):
    if path.is_symlink() or not path.is_dir() or not VERSION.fullmatch(path.name):
        return None
    if not regular(path / "release.json") or not regular(path / "SHA256SUMS"):
        return None
    try:
        meta = json.loads((path / "release.json").read_text())
        if meta.get("version") != path.name:
            return None
        stamp = dt.datetime.fromisoformat(meta["built_at"].replace("Z", "+00:00")).timestamp()
    except (ValueError, KeyError, TypeError):
        return None
    size = 0
    for root, dirs, files in os.walk(path, followlinks=False):
        for name in dirs + files:
            p = Path(root) / name
            if p.is_symlink():
                return None
            if p.is_file():
                size += p.stat().st_size
    return {"name": path.name, "stamp": stamp, "bytes": size, "source_commit": meta.get("source_commit"),
            "fingerprint": fingerprint(path), "metadata": fingerprint(path / "release.json")}


def backup_selection(dumps, now):
    ordered = sorted(dumps, key=lambda p: (-p.stat().st_mtime, p.name))
    keep = {p.name for p in ordered[:3]}
    buckets = set()
    for p in ordered:
        stamp = p.stat().st_mtime
        date = dt.datetime.fromtimestamp(stamp, dt.timezone.utc)
        age = (now - stamp) / 86400
        kind, key = ("day", date.strftime("%Y-%m-%d")) if age <= 7 else (
            ("week", date.strftime("%G-%V")) if age <= 28 else ("month", date.strftime("%Y-%m")))
        if (kind, key) not in buckets:
            keep.add(p.name)
            buckets.add((kind, key))
    return keep


def make_plan(site, backups=None, pins=None, now=None):
    now = time.time() if now is None else now
    pins = pins or {}
    if site.is_symlink() or not site.is_dir() or (site / "releases").is_symlink():
        raise ValueError("site and releases must be real directories")
    site = site.resolve(strict=True)
    releases_root = (site / "releases").resolve(strict=True)
    if releases_root.parent != site:
        raise ValueError("releases escaped site")
    current = (site / "current").resolve(strict=True)
    if current.parent != releases_root or not current.is_dir():
        raise ValueError("current must resolve to an immediate release directory")
    releases, protected = [], []
    for p in releases_root.iterdir():
        item = release_info(p)
        if item is None:
            protected.append(p.name)
        else:
            releases.append(item)
    releases.sort(key=lambda r: (-r["stamp"], r["name"]))
    release_keep = {r["name"] for r in releases[:5]} | {current.name} | set(pins.get("releases", []))
    kept_backups, delete_backups = [], []
    if backups is not None:
        if backups.is_symlink() or not backups.is_dir():
            raise ValueError("backup root must be a real directory")
        backups = backups.resolve(strict=True)
        if backups == site or backups in site.parents or site in backups.parents:
            raise ValueError("backup and release roots must be separate")
        dumps = [p for p in backups.glob("*.dump") if regular(p)]
        complete = [p for p in dumps if regular(p.with_suffix(".env")) and p.stat().st_size > 0]
        selected = backup_selection(complete, now) | set(pins.get("backups", []))
        for p in dumps:
            # Missing key material, incomplete files and historical layouts are retained.
            keep = p.name in selected or p not in complete
            files = [p] + [p.with_suffix(s) for s in (".env", ".meta.json") if regular(p.with_suffix(s))]
            item = {"name": p.name, "complete": p in complete,
                    "files": [{"name": f.name, "bytes": f.stat().st_size, "fingerprint": fingerprint(f)} for f in files]}
            (kept_backups if keep else delete_backups).append(item)
            if keep:
                meta = p.with_suffix(".meta.json")
                if regular(meta):
                    backend = json.loads(meta.read_text()).get("backend_release")
                    if not backend or not VERSION.fullmatch(backend) or not (releases_root / backend).is_dir():
                        raise ValueError("backup metadata refers to an unavailable release")
                    release_keep.add(backend)
                else:
                    # Legacy backups predate explicit version metadata. Retain the
                    # destination named in the backup and the two preceding builds.
                    match = re.fullmatch(r"before-(v.+)-\d{8}T\d{6}Z\.dump", p.name)
                    if match:
                        release_keep.add(match[1])
                    prior = [r for r in releases if r["stamp"] <= p.stat().st_mtime]
                    release_keep.update(r["name"] for r in prior[:2])
    return {"policy": 1, "site": str(site), "backups": str(backups) if backups else None,
            "current": current.name, "keep_releases": sorted(release_keep), "protected": sorted(protected),
            "keep_backups": sorted(kept_backups, key=lambda r: r["name"]),
            "delete_backups": sorted(delete_backups, key=lambda r: r["name"]),
            "delete_releases": [r for r in releases if r["name"] not in release_keep]}


def verify_backup(path):
    with path.open("rb") as source:
        result = subprocess.run(["docker", "compose", "-f", "/etc/glorynavy/compose.database.yaml",
                                 "exec", "-T", "db", "pg_restore", "--file=/dev/null"],
                                stdin=source, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=900)
    if result.returncode:
        raise ValueError("retained backup archive verification failed: " + path.name)


def apply_plan(plan, pins, verify=verify_backup):
    site = Path(plan["site"])
    backups = Path(plan["backups"]) if plan["backups"] else None
    if make_plan(site, backups, pins) != plan:
        raise ValueError("inventory changed; generate a fresh plan")
    for item in plan["keep_backups"]:
        if item["complete"]:
            verify(backups / item["name"])
    for name in plan["keep_releases"]:
        p = site / "releases" / name
        if p.is_dir() and regular(p / "SHA256SUMS"):
            result = subprocess.run(["sha256sum", "--quiet", "-c", "SHA256SUMS"], cwd=p,
                                    stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=180)
            if result.returncode:
                raise ValueError("retained release verification failed: " + name)
    if make_plan(site, backups, pins) != plan:
        raise ValueError("inventory changed during verification")
    # Delete only exact, inventoried immediate children, under the deployment lock.
    for item in plan["delete_backups"]:
        for f in item["files"]:
            p = backups / f["name"]
            if p.parent != backups or not regular(p) or fingerprint(p) != f["fingerprint"]:
                raise ValueError("backup changed before removal")
            p.unlink()
    for item in plan["delete_releases"]:
        p = site / "releases" / item["name"]
        if p.is_symlink() or p.resolve().parent != site / "releases" or p == (site / "current").resolve():
            raise ValueError("unsafe release removal")
        if release_info(p) != item:
            raise ValueError("release changed before removal")
        shutil.rmtree(p)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--role", choices=["app", "edge"], required=True)
    parser.add_argument("--apply", action="store_true")
    parser.add_argument("--pins", default="/etc/glorynavy/storage-retention-pins.json")
    parser.add_argument("--report-dir", default="/var/log/glorynavy-storage-retention")
    args = parser.parse_args()
    site = Path("/opt/glorynavy" if args.role == "app" else "/opt/1panel/www/sites/seat.kisectool.com")
    backups = Path("/var/backups/glorynavy") if args.role == "app" else None
    pins_path = Path(args.pins)
    pins = json.loads(pins_path.read_text()) if pins_path.exists() else {}
    if set(pins) - {"releases", "backups"} or any(not isinstance(v, list) or any(not isinstance(n, str) or Path(n).name != n for n in v) for v in pins.values()):
        raise ValueError("invalid retention pins")
    with open("/run/lock/glorynavy-storage.lock", "w") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        plan = make_plan(site, backups, pins)
        report_dir = Path(args.report_dir)
        report_dir.mkdir(mode=0o700, parents=True, exist_ok=True)
        stamp = dt.datetime.now(dt.timezone.utc).strftime("%Y%m%dT%H%M%S.%fZ")
        report = report_dir / (stamp + "-" + args.role + ".json")
        with report.open("x") as f:
            os.chmod(report, 0o600)
            json.dump({"apply": args.apply, "plan": plan}, f, indent=2)
        if args.apply:
            apply_plan(plan, pins)
            with report.open("w") as f:
                json.dump({"apply": True, "completed": True, "plan": plan}, f, indent=2)
        print(json.dumps({"report": str(report), "applied": args.apply,
                          "release_count": len(plan["delete_releases"]),
                          "backup_count": len(plan["delete_backups"]),
                          "candidate_bytes": sum(r["bytes"] for r in plan["delete_releases"]) + sum(f["bytes"] for r in plan["delete_backups"] for f in r["files"])}))


if __name__ == "__main__":
    os.umask(0o077)
    main()
