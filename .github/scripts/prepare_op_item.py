#!/usr/bin/env python3

import argparse
import json
from pathlib import Path


def load_cookies_json(cookies_path: Path) -> str:
    cookies = json.loads(cookies_path.read_text(encoding="utf-8"))
    return json.dumps(cookies, separators=(",", ":"), sort_keys=True)


def note_field_payload(cookies_json: str) -> dict[str, str]:
    return {
        "id": "notesPlain",
        "label": "notesPlain",
        "type": "STRING",
        "value": cookies_json,
    }


def update_existing_item(item_path: Path, cookies_json: str) -> None:
    item = json.loads(item_path.read_text(encoding="utf-8"))

    for field in item.get("fields", []):
        if field.get("id") == "notesPlain" or field.get("label") == "notesPlain":
            field["value"] = cookies_json
            break
    else:
        item.setdefault("fields", []).append(note_field_payload(cookies_json))

    item_path.write_text(json.dumps(item), encoding="utf-8")
    item_path.chmod(0o600)


def prepare_new_item(item_path: Path, title: str, cookies_json: str) -> None:
    item = json.loads(item_path.read_text(encoding="utf-8"))
    item["title"] = title

    for field in item.get("fields", []):
        if field.get("id") == "notesPlain" or field.get("label") == "notesPlain":
            field["value"] = cookies_json
            break
    else:
        item.setdefault("fields", []).append(note_field_payload(cookies_json))

    item_path.write_text(json.dumps(item), encoding="utf-8")
    item_path.chmod(0o600)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--item-path", required=True)
    parser.add_argument("--cookies-path", default="cookies.json")
    parser.add_argument("--title")
    parser.add_argument("--mode", choices=["update", "create"], required=True)
    args = parser.parse_args()

    item_path = Path(args.item_path)
    cookies_path = Path(args.cookies_path)
    cookies_json = load_cookies_json(cookies_path)

    if args.mode == "update":
        update_existing_item(item_path, cookies_json)
        return

    prepare_new_item(item_path, args.title or "", cookies_json)


if __name__ == "__main__":
    main()
