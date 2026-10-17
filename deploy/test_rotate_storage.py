import datetime as dt
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import time
import unittest

spec = importlib.util.spec_from_file_location("retention", Path(__file__).with_name("rotate-storage.py"))
retention = importlib.util.module_from_spec(spec)
spec.loader.exec_module(retention)


class RotationTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.site = self.root / "site"
        (self.site / "releases").mkdir(parents=True)
        self.backups = self.root / "backups"
        self.backups.mkdir()
        self.now = time.time()
        for i in range(10):
            self.release(i)
        (self.site / "current").symlink_to("releases/v0.1.0-test-9")

    def release(self, n):
        p = self.site / "releases" / f"v0.1.0-test-{n}"
        p.mkdir()
        stamp = dt.datetime.fromtimestamp(self.now - (10 - n) * 86400, dt.timezone.utc).isoformat()
        (p / "release.json").write_text(json.dumps({"version": p.name, "built_at": stamp}))
        (p / "binary").write_bytes(b"program")
        (p / "SHA256SUMS").write_text("".join(hashlib.sha256(f.read_bytes()).hexdigest()+"  "+f.name+"\n" for f in [p / "binary", p / "release.json"]))
        return p

    def backup(self, name, age, complete=True, backend=None):
        p = self.backups / (name + ".dump")
        p.write_bytes(b"archive")
        os.utime(p, (self.now-age*86400, self.now-age*86400))
        if complete:
            p.with_suffix(".env").write_bytes(b"key material")
        if backend:
            p.with_suffix(".meta.json").write_text(json.dumps({"backend_release": backend}))
        return p

    def test_keeps_current_latest_five_and_pins(self):
        (self.site / "current").unlink()
        (self.site / "current").symlink_to("releases/v0.1.0-test-0")
        plan = retention.make_plan(self.site, pins={"releases": ["v0.1.0-test-1"]})
        self.assertEqual({r["name"] for r in plan["delete_releases"]}, {f"v0.1.0-test-{i}" for i in [2, 3, 4]})

    def test_backup_buckets_and_latest_three_keep_complete_pairs(self):
        for i, age in enumerate([.1, .2, .3, .4, 2, 2.1, 9, 9.1, 40, 40.1]):
            self.backup(f"snapshot-{i}", age)
        incomplete = self.backup("incomplete", .01, complete=False)
        plan = retention.make_plan(self.site, self.backups)
        kept = {r["name"] for r in plan["keep_backups"]}
        self.assertTrue({"snapshot-0.dump", "snapshot-1.dump", "snapshot-2.dump", incomplete.name} <= kept)
        self.assertTrue(plan["delete_backups"])
        self.assertTrue(all(len(r["files"]) == 2 for r in plan["delete_backups"]))

    def test_backup_metadata_protects_restore_release(self):
        self.backup("snapshot", 2, backend="v0.1.0-test-0")
        plan = retention.make_plan(self.site, self.backups)
        self.assertIn("v0.1.0-test-0", plan["keep_releases"])

    def test_preserves_unknown_and_symlink_releases(self):
        old = self.site / "releases" / "v0.1.0-test-0"
        (old / "external").symlink_to(self.backups, target_is_directory=True)
        unknown = self.site / "releases" / "unfinished"
        unknown.mkdir()
        plan = retention.make_plan(self.site)
        self.assertIn(old.name, plan["protected"])
        self.assertIn(unknown.name, plan["protected"])

    def test_stale_plan_never_deletes(self):
        plan = retention.make_plan(self.site)
        (self.site / "current").unlink()
        (self.site / "current").symlink_to("releases/v0.1.0-test-0")
        with self.assertRaises(ValueError):
            retention.apply_plan(plan, {})
        self.assertEqual(len(list((self.site / "releases").iterdir())), 10)

    def test_backup_verification_failure_never_deletes(self):
        self.backup("snapshot", .1)
        plan = retention.make_plan(self.site, self.backups)
        def fail(_):
            raise ValueError("corrupt archive")
        with self.assertRaises(ValueError):
            retention.apply_plan(plan, {}, verify=fail)
        self.assertEqual(len(list((self.site / "releases").iterdir())), 10)

    def test_apply_removes_only_candidates_and_is_idempotent(self):
        plan = retention.make_plan(self.site)
        retention.apply_plan(plan, {})
        self.assertEqual(len(list((self.site / "releases").iterdir())), 5)
        next_plan = retention.make_plan(self.site)
        self.assertEqual(next_plan["delete_releases"], [])
        retention.apply_plan(next_plan, {})

    def test_rejects_escaped_current_and_symlink_roots(self):
        (self.site / "current").unlink()
        (self.site / "current").symlink_to(self.backups, target_is_directory=True)
        with self.assertRaises(ValueError):
            retention.make_plan(self.site)
        alias = self.root / "alias"
        alias.symlink_to(self.site, target_is_directory=True)
        with self.assertRaises(ValueError):
            retention.make_plan(alias)

    def test_corrupt_retained_release_stops_all_deletion(self):
        p = self.site / "releases" / "v0.1.0-test-9" / "binary"
        p.write_bytes(b"changed")
        plan = retention.make_plan(self.site)
        with self.assertRaises(ValueError):
            retention.apply_plan(plan, {})
        self.assertEqual(len(list((self.site / "releases").iterdir())), 10)

    def test_apply_deletes_exact_backup_pairs_and_preserves_unpaired_files(self):
        for i in range(10):
            self.backup(f"snapshot-{i}", .1+i*.01, backend="v0.1.0-test-0")
        incomplete = self.backup("incomplete", .1, complete=False)
        sentinel = self.backups / "unrelated.env"
        sentinel.write_bytes(b"keep")
        plan = retention.make_plan(self.site, self.backups)
        self.assertTrue(plan["delete_backups"])
        retention.apply_plan(plan, {}, verify=lambda _: None)
        for item in plan["delete_backups"]:
            self.assertTrue(all(not (self.backups / f["name"]).exists() for f in item["files"]))
        for item in plan["keep_backups"]:
            self.assertTrue(all((self.backups / f["name"]).exists() for f in item["files"]))
        self.assertTrue(incomplete.exists() and sentinel.exists())


if __name__ == "__main__":
    unittest.main()
