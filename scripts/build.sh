#!/usr/bin/env bash
# Reproducible production build: web (npm ci, tsc, Parcel) then Go binaries
# with the embedded UI. Output: ./bin/pawn-ink-*.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
make build
ls -l bin/
