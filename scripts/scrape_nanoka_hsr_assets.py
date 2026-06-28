"""
Download all hsr.nanoka.cc image assets discovered via JS bundle inspection.

Output mirrors the CDN structure under nanoka_hsr/<version>/assets/hsr/...
Resumable: existing files are skipped. Failures are recorded for retry.

Resource paths discovered from _app/immutable/nodes/*.js (v4.3.54):

  avatarroundicon/{id}.webp           Round portrait (small chip)
  avatarshopicon/{id}.webp            Shop card (medium)
  avataricon/avatar/{id}.webp         In-team icon
  avatardrawcard/{id}.webp            Gacha splash art (largest)
  rank/_dependencies/textures/{id}/{id}_Rank_{1..6}.webp  Eidolon art
  skillicons/{iconName-without-.png}.webp  Skills/Traces/Eidolon-Icons/Stat-Icons
  pathicon/{baseType_lowercase}.webp  Path icons (shared, 9 paths)
  element/{damageType_lowercase}.webp Element icons (shared, 7)
  lightconemediumicon/{id}.webp       Light-cone medium icon
  lightconemaxfigures/{id}.webp       Light-cone full art
  relicfigures/IconRelic{slot}.webp   Slot icons (Head/Hands/Body/Foot/Neck/Goods)
  itemfigures/{stem}.webp             Items, relics sets (set_id), materials
"""
from __future__ import annotations

import json
import re
import sys
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path

import requests

UA = "Mozilla/5.0 (compatible; nanoka-asset-mirror/1.0)"
SESSION = requests.Session()
SESSION.headers["User-Agent"] = UA
ROOT = Path("nanoka_hsr")
ASSET_BASE = "https://static.nanoka.cc/assets/hsr"
OG_BASE = "https://hsr.nanoka.cc/character"


def resolve_version() -> str:
    html = SESSION.get("https://hsr.nanoka.cc/character", timeout=30).text
    m = re.search(r"static\.nanoka\.cc/hsr/(\d+\.\d+\.\d+)/", html)
    if not m:
        sys.exit("version not found")
    return m.group(1)


def strip_ext(name: str) -> str:
    """JSON gives 'SkillIcon_1309_Normal.png'; CDN wants 'SkillIcon_1309_Normal'."""
    return re.sub(r"\.(png|jpg|jpeg|webp)$", "", name, flags=re.I)


def build_jobs(version: str) -> list[tuple[str, Path]]:
    """Produce (url, dest_path) pairs from the master JSONs already on disk."""
    root = ROOT / version
    out_root = root / "assets" / "hsr"
    jobs: list[tuple[str, Path]] = []

    # ------------------------------------------------------------------
    # Characters: 4 portrait styles + 6 eidolon images + OG banner
    # ------------------------------------------------------------------
    chars: dict = json.loads((root / "character.json").read_text("utf-8"))
    for cid in chars:
        for folder in ("avatarroundicon", "avatarshopicon", "avatardrawcard"):
            jobs.append((f"{ASSET_BASE}/{folder}/{cid}.webp",
                         out_root / folder / f"{cid}.webp"))
        jobs.append((f"{ASSET_BASE}/avataricon/avatar/{cid}.webp",
                     out_root / "avataricon" / "avatar" / f"{cid}.webp"))
        for rank in range(1, 7):
            jobs.append((f"{ASSET_BASE}/rank/_dependencies/textures/{cid}/{cid}_Rank_{rank}.webp",
                         out_root / "rank" / "_dependencies" / "textures" / cid / f"{cid}_Rank_{rank}.webp"))
        jobs.append((f"{OG_BASE}/{cid}/og.png",
                     out_root / "og" / f"{cid}.png"))

    # Skills / traces / stat-icons from detail JSONs (any one language is fine; icons are same).
    seen_icons: set[str] = set()
    detail_dir = root / "en" / "character"
    for fp in sorted(detail_dir.glob("*.json")):
        d = json.loads(fp.read_text("utf-8"))
        for sk in d.get("skills", {}).values():
            ic = sk.get("icon")
            if ic: seen_icons.add(strip_ext(ic))
        for rk in d.get("ranks", {}).values():
            ic = rk.get("icon")
            if ic: seen_icons.add(strip_ext(ic))
        for pt in d.get("skill_trees", {}).values():
            for lvl in pt.values() if isinstance(pt, dict) else []:
                if isinstance(lvl, dict):
                    ic = lvl.get("icon")
                    if ic: seen_icons.add(strip_ext(ic))
    for name in sorted(seen_icons):
        jobs.append((f"{ASSET_BASE}/skillicons/{name}.webp",
                     out_root / "skillicons" / f"{name}.webp"))

    # ------------------------------------------------------------------
    # Light cones: medium icon + max figure
    # ------------------------------------------------------------------
    lcs: dict = json.loads((root / "lightcone.json").read_text("utf-8"))
    for lcid in lcs:
        jobs.append((f"{ASSET_BASE}/lightconemediumicon/{lcid}.webp",
                     out_root / "lightconemediumicon" / f"{lcid}.webp"))
        jobs.append((f"{ASSET_BASE}/lightconemaxfigures/{lcid}.webp",
                     out_root / "lightconemaxfigures" / f"{lcid}.webp"))

    # ------------------------------------------------------------------
    # Relic sets: figure stored under itemfigures/{set_id}
    # ------------------------------------------------------------------
    rels: dict = json.loads((root / "relicset.json").read_text("utf-8"))
    for rid in rels:
        jobs.append((f"{ASSET_BASE}/itemfigures/{rid}.webp",
                     out_root / "itemfigures" / f"{rid}.webp"))

    # ------------------------------------------------------------------
    # Items (materials etc.) — image filename is stem of item_figure_icon_path
    # ------------------------------------------------------------------
    items: dict = json.loads((root / "en" / "item.json").read_text("utf-8"))
    for it in items.values():
        p = it.get("item_figure_icon_path")
        if not p: continue
        stem = strip_ext(Path(p).name)
        jobs.append((f"{ASSET_BASE}/itemfigures/{stem}.webp",
                     out_root / "itemfigures" / f"{stem}.webp"))

    # ------------------------------------------------------------------
    # Shared small icons (paths, elements, relic slots)
    # ------------------------------------------------------------------
    paths = ["knight", "mage", "priest", "rogue", "shaman", "warlock", "warrior", "memory"]
    for p in paths:
        jobs.append((f"{ASSET_BASE}/pathicon/{p}.webp",
                     out_root / "pathicon" / f"{p}.webp"))

    elements = ["fire", "ice", "imaginary", "physical", "quantum", "thunder", "wind"]
    for e in elements:
        jobs.append((f"{ASSET_BASE}/element/{e}.webp",
                     out_root / "element" / f"{e}.webp"))

    slots = ["Head", "Hands", "Body", "Foot", "Neck", "Goods"]
    for s in slots:
        jobs.append((f"{ASSET_BASE}/relicfigures/IconRelic{s}.webp",
                     out_root / "relicfigures" / f"IconRelic{s}.webp"))

    # de-duplicate
    seen = set()
    uniq = []
    for url, dest in jobs:
        if url in seen: continue
        seen.add(url)
        uniq.append((url, dest))
    return uniq


def download(url: str, dest: Path) -> int:
    if dest.exists() and dest.stat().st_size > 0:
        return 0  # skip
    dest.parent.mkdir(parents=True, exist_ok=True)
    r = SESSION.get(url, timeout=30)
    if r.status_code != 200:
        raise RuntimeError(f"HTTP {r.status_code}")
    if len(r.content) < 200:  # nanoka 404 page is ~146 bytes
        raise RuntimeError(f"suspiciously small ({len(r.content)}B)")
    dest.write_bytes(r.content)
    return len(r.content)


def main(workers: int = 8) -> None:
    version = resolve_version()
    print(f"version: {version}")
    jobs = build_jobs(version)
    todo = [(u, d) for u, d in jobs if not (d.exists() and d.stat().st_size > 0)]
    print(f"jobs total: {len(jobs)}, to fetch: {len(todo)}")

    failed: list[tuple[str, str]] = []
    bytes_total = 0
    t0 = time.time()
    with ThreadPoolExecutor(max_workers=workers) as pool:
        futs = {pool.submit(download, u, d): (u, d) for u, d in todo}
        for i, fut in enumerate(as_completed(futs), 1):
            url, dest = futs[fut]
            try:
                bytes_total += fut.result()
            except Exception as exc:  # noqa: BLE001
                failed.append((url, str(exc)))
            if i % 100 == 0 or i == len(futs):
                mb = bytes_total / 1_000_000
                elapsed = time.time() - t0
                print(f"  {i}/{len(futs)}  +{mb:.1f} MB  ({elapsed:.0f}s)")

    if failed:
        log = ROOT / version / "failed_assets.txt"
        log.write_text("\n".join(f"{u}  -- {e}" for u, e in failed), "utf-8")
        print(f"\n{len(failed)} failed (see {log})")
    print(f"total downloaded this run: {bytes_total / 1_000_000:.1f} MB")


if __name__ == "__main__":
    main()
