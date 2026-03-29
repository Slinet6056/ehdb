#!/usr/bin/env python3

import json
import os
from pathlib import Path


def main() -> None:
    raw = os.environ["EHENTAI_COOKIES_JSON"]
    json.loads(raw)

    path = Path("cookies.json")
    path.write_text(raw + ("\n" if not raw.endswith("\n") else ""), encoding="utf-8")
    path.chmod(0o600)


if __name__ == "__main__":
    main()
