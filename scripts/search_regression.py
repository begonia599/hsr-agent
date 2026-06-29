#!/usr/bin/env python3
"""Run Chinese search regression cases against the HTTP API."""

from __future__ import annotations

import argparse
import json
import sys
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
DEFAULT_CASES = ROOT / "docs" / "search_regression.json"


def load_cases(path: Path) -> list[dict[str, Any]]:
    with path.open("r", encoding="utf-8") as fh:
        data = json.load(fh)
    if not isinstance(data, list):
        raise ValueError("regression file must contain a JSON array")
    return data


def fetch_json(url: str, timeout: float) -> Any:
    req = urllib.request.Request(url, headers={"Accept": "application/json"})
    with urllib.request.urlopen(req, timeout=timeout) as res:
        return json.loads(res.read().decode("utf-8"))


def item_names(payload: Any) -> list[str]:
    items = payload.get("items", payload) if isinstance(payload, dict) else payload
    if not isinstance(items, list):
        return []
    names: list[str] = []
    for item in items:
        if isinstance(item, dict):
            name = str(item.get("name_zh") or "")
            if name:
                names.append(name)
    return names


def semantic_url(base_url: str, case: dict[str, Any], rerank: str | None) -> str:
    params = {
        "q": case["query"],
        "kind": case.get("kind", "character"),
        "limit": str(case.get("limit", 10)),
        "include_meta": "true",
    }
    if "embedding_model_id" in case:
        params["embedding_model_id"] = case["embedding_model_id"]
    if rerank is not None:
        params["rerank"] = rerank
    return base_url.rstrip("/") + "/api/search/semantic?" + urllib.parse.urlencode(params)


def run_case(base_url: str, case: dict[str, Any], timeout: float, rerank: str | None) -> tuple[bool, str]:
    url = semantic_url(base_url, case, rerank)
    payload = fetch_json(url, timeout)
    names = item_names(payload)
    expected = [str(item) for item in case.get("expected_any", [])]
    matched = [name for name in expected if name in names]
    ok = bool(matched)
    detail = f"{case['query']} [{case.get('kind', 'character')}] expected_any={expected} top={names[:8]}"
    return ok, detail


def main() -> int:
    if hasattr(sys.stdout, "reconfigure"):
        sys.stdout.reconfigure(encoding="utf-8")
        sys.stderr.reconfigure(encoding="utf-8")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", default="http://127.0.0.1:8080")
    parser.add_argument("--cases", type=Path, default=DEFAULT_CASES)
    parser.add_argument("--timeout", type=float, default=30)
    parser.add_argument("--rerank", choices=["true", "false"], help="Override rerank query parameter.")
    args = parser.parse_args()

    cases = load_cases(args.cases)
    failures = 0
    for index, case in enumerate(cases, 1):
        try:
            ok, detail = run_case(args.base_url, case, args.timeout, args.rerank)
        except urllib.error.HTTPError as exc:
            failures += 1
            body = exc.read().decode("utf-8", errors="replace")
            print(f"not ok {index} - HTTP {exc.code}: {body}")
            continue
        except Exception as exc:
            failures += 1
            print(f"not ok {index} - {exc}")
            continue
        if ok:
            print(f"ok {index} - {detail}")
        else:
            failures += 1
            print(f"not ok {index} - {detail}")

    passed = len(cases) - failures
    print(f"summary: passed={passed} failed={failures} total={len(cases)}")
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
