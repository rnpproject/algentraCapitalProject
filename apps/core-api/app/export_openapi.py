"""Export the checked-in API contract without starting server workers."""

import json
from pathlib import Path

from .main import app


def main() -> None:
    repository_root = Path(__file__).resolve().parents[3]
    contract_path = repository_root / "packages" / "contracts" / "openapi.json"
    contract_path.parent.mkdir(parents=True, exist_ok=True)
    contract_path.write_text(json.dumps(app.openapi(), indent=2) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
