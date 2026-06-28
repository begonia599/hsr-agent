"""
Scrape character data from hsr.nanoka.cc.

Data flow:
  1. Resolve current version by fetching the SPA shell and reading the inlined CDN url.
  2. Fetch character.json (the master index, ~95 entries).
  3. For each character id, fetch <lang>/character/<id>.json for every requested language.
  4. Write everything under ./nanoka_hsr/<version>/.

The CDN has no auth, no rate limit beyond normal, but we still throttle (4 concurrent,
small jitter) to be polite. Resumable: existing files are skipped.
"""
from __future__ import annotations

import json
import re
import sys
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path

import requests

UA = "Mozilla/5.0 (compatible; nanoka-data-mirror/1.0)"
LANGS = ("en", "zh", "ko", "ja")
ROOT = Path("nanoka_hsr")
SESSION = requests.Session()
SESSION.headers["User-Agent"] = UA


def resolve_version() -> str:
    """Read https://hsr.nanoka.cc/character and pluck the version from the inline URL."""
    html = SESSION.get("https://hsr.nanoka.cc/character", timeout=30).text
    m = re.search(r"static\.nanoka\.cc/hsr/(\d+\.\d+\.\d+)/", html)
    if not m:
        sys.exit("could not find version in SPA shell")
    return m.group(1)


def get_json(url: str, dest: Path) -> dict:
    if dest.exists():
        return json.loads(dest.read_text("utf-8"))
    r = SESSION.get(url, timeout=30)
    r.raise_for_status()
    dest.parent.mkdir(parents=True, exist_ok=True)
    dest.write_text(r.text, encoding="utf-8")
    return r.json()


def main(langs: tuple[str, ...] = LANGS, workers: int = 4) -> None:
    version = resolve_version()
    print(f"version: {version}")
    base = f"https://static.nanoka.cc/hsr/{version}"
    out = ROOT / version
    out.mkdir(parents=True, exist_ok=True)

    # 1. Index: every character id is a top-level key of character.json
    index = get_json(f"{base}/character.json", out / "character.json")
    ids = sorted(index.keys(), key=int)
    print(f"characters: {len(ids)}")

    # 2. Also grab the smaller siblings while we're here (lightcone / relicset / item).
    sidecars = [
        f"{base}/lightcone.json",
        f"{base}/relicset.json",
    ] + [f"{base}/{lang}/item.json" for lang in langs]
    for url in sidecars:
        rel = url.removeprefix(base + "/")
        get_json(url, out / rel)
        print(f"  ok  {rel}")

    # 3. Detail JSONs in every language, in parallel.
    jobs = [
        (lang, cid, f"{base}/{lang}/character/{cid}.json", out / lang / "character" / f"{cid}.json")
        for lang in langs
        for cid in ids
    ]
    todo = [j for j in jobs if not j[3].exists()]
    print(f"detail files to fetch: {len(todo)} (of {len(jobs)})")

    failed: list[tuple[str, str]] = []
    with ThreadPoolExecutor(max_workers=workers) as pool:
        futures = {pool.submit(get_json, url, dest): (lang, cid) for lang, cid, url, dest in todo}
        for i, fut in enumerate(as_completed(futures), 1):
            lang, cid = futures[fut]
            try:
                fut.result()
                if i % 25 == 0 or i == len(futures):
                    print(f"  {i}/{len(futures)}  {lang}/{cid}")
            except Exception as exc:  # noqa: BLE001
                print(f"  fail {lang}/{cid}: {exc}", file=sys.stderr)
                failed.append((lang, cid))
            time.sleep(0.05)  # gentle throttle

    if failed:
        print(f"\n{len(failed)} failed — re-run to retry (existing files are kept).")
    else:
        print("\nall done.")


if __name__ == "__main__":
    main()
