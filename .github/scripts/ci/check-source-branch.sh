#!/usr/bin/env bash
# check-source-branch.sh — main への PR のソースブランチを検証する。
#
# BRANCHING.md で「main へのマージ元は release/* と hotfix/* のみ」と定めているが
# GitHub Rulesets にはソースブランチ制限のネイティブ機能がないため CI で強制する。
# release は release-tag.yaml がブランチ名からバージョンを取るため SemVer 形式を厳格に検証する。
set -euo pipefail

HEAD_REF="${1:-}"

if [ -z "$HEAD_REF" ]; then
  echo "::error::source branch is empty"
  exit 1
fi

if [[ "$HEAD_REF" =~ ^release/v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "source branch '$HEAD_REF' matches release/vX.Y.Z"
  exit 0
fi

if [[ "$HEAD_REF" =~ ^hotfix/.+ ]]; then
  echo "source branch '$HEAD_REF' matches hotfix/*"
  exit 0
fi

if [[ "$HEAD_REF" =~ ^release/ ]]; then
  echo "::error::release branch '$HEAD_REF' does not follow the 'release/vX.Y.Z' naming (see docs/BRANCHING.md)"
  exit 1
fi

echo "::error::main へのマージは 'release/vX.Y.Z' または 'hotfix/*' からのみ許可されています (got: $HEAD_REF)"
exit 1
