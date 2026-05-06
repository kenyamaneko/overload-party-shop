#!/usr/bin/env python3
"""data/models.yaml から packages/api-shop / internal/domain に Go 型を生成する.

共通基盤 `overload-party-codegen-tools` の `CodegenRunner` を使う。shop は
inter-service event のように同じ JSON 形状を両層に出したいケースがあり、
`targets: [domain, wire]` のリスト指定で複数 target に同時出力できる。

実行: python3 scripts/generate_types.py
"""

from __future__ import annotations

import sys
from pathlib import Path

from codegen_tools import CodegenRunner, GoConstStyle, GoStyle, GoTarget

REPO_ROOT = Path(__file__).resolve().parent.parent
MODELS_YAML = REPO_ROOT / "data" / "models.yaml"


def main() -> int:
    style = GoStyle(
        tag_keys=("json",),
        type_comment_multiline=True,
        const_style=GoConstStyle(
            type_annotation=False,
            quote_string_only=True,
        ),
    )

    runner = CodegenRunner(
        models_yaml=MODELS_YAML,
        repo_root=REPO_ROOT,
        targets={
            "domain": GoTarget(
                out_dir=REPO_ROOT / "internal" / "domain",
                package="domain",
                emit_tags=("json",),
            ),
            "wire": GoTarget(
                out_dir=REPO_ROOT / "packages" / "api-shop",
                package="apishop",
                emit_tags=("json",),
            ),
        },
        style=style,
        single_target_field="target",
        multi_target_field="targets",
        trailing_blank_line=True,
    )
    return runner.run()


if __name__ == "__main__":
    sys.exit(main())
