#!/usr/bin/env bash
# Puts the EDB PostgreSQL installer that SemiBase-Setup.exe carries into Installer/payload/.
# The constants are docs/architecture/installer.md#the-pinned-payload.
set -euo pipefail

readonly VERSION=17.11-5
readonly URL="https://get.enterprisedb.com/postgresql/postgresql-${VERSION}-windows-x64.exe"
readonly SHA256=612c7f5400a003aaffa0372cb273eed52278f93c680ab911eb9cb2229e329072

# A fixed name, so a pin bump leaves Installer/SemiBase.iss, which names the same file, unchanged
readonly FILE_NAME=postgresql-17-windows-x64.exe
PAYLOAD_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/payload"
readonly PAYLOAD_DIR
readonly PART_NAME="${FILE_NAME}.part"

matches_pin() {
  local name=$1
  (cd "$PAYLOAD_DIR" && printf '%s *%s\n' "$SHA256" "$name" | sha256sum -c --status)
}

main() {
  if [[ $# -ne 0 ]]; then
    echo "usage: fetch-postgresql.sh" >&2
    exit 2
  fi
  local tool
  for tool in curl sha256sum; do
    command -v "$tool" >/dev/null || { echo "$tool required" >&2; exit 1; }
  done

  mkdir -p "$PAYLOAD_DIR"
  if [[ -e "$PAYLOAD_DIR/$FILE_NAME" ]]; then
    if matches_pin "$FILE_NAME"; then
      echo "$PAYLOAD_DIR/$FILE_NAME matches the pinned SHA256, nothing to download" >&2
      return 0
    fi
    # A download lands under its final name only after it matched, so this file was changed.
    echo "$PAYLOAD_DIR/$FILE_NAME does not match SHA256 $SHA256; delete it and run again" >&2
    exit 1
  fi

  trap 'rm -f "$PAYLOAD_DIR/$PART_NAME"' EXIT
  echo "downloading $URL" >&2
  curl -fsSL --retry 3 -o "$PAYLOAD_DIR/$PART_NAME" "$URL"
  if ! matches_pin "$PART_NAME"; then
    echo "$URL does not match the pinned SHA256 $SHA256:" \
      "the server changed the file, so the pin needs a new measurement" >&2
    exit 1
  fi
  mv "$PAYLOAD_DIR/$PART_NAME" "$PAYLOAD_DIR/$FILE_NAME"
  echo "$PAYLOAD_DIR/$FILE_NAME downloaded and verified" >&2
}

main "$@"
