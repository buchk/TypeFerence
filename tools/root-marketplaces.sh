#!/bin/sh
# Derive the repository-root marketplace indexes from the committed build of
# agents/typeference (ADR-0008): each client's index from
# dist-plugin/agent-plugin/, with every plugin path prefixed by that committed
# path, so Copilot, Claude Code, and Codex each recognize this repository as a
# marketplace. Run from the repository root.
#
#   tools/root-marketplaces.sh           write the root indexes
#   tools/root-marketplaces.sh --check   fail if a root index has drifted
set -e
target=dist-plugin/agent-plugin
prefix="./$target/"
status=0
for index in .github/plugin/marketplace.json .claude-plugin/marketplace.json .agents/plugins/marketplace.json; do
  built="$target/$index"
  if [ ! -f "$built" ]; then
    if [ -f "$index" ]; then
      echo "$index exists, but the build emits no $built" >&2
      status=1
    fi
    continue
  fi
  derived=$(sed -e "s|\"source\": \"\\./|\"source\": \"$prefix|" -e "s|\"path\": \"\\./|\"path\": \"$prefix|" "$built")
  if [ "$1" = --check ]; then
    if ! printf '%s\n' "$derived" | cmp -s - "$index"; then
      echo "$index has drifted from $built; run make selfhost" >&2
      status=1
    fi
  else
    mkdir -p "$(dirname "$index")"
    printf '%s\n' "$derived" > "$index"
  fi
done
exit $status
