#!/bin/bash
# Compila la imagen Docker de produccion.
#
# Igual que build-docker-dev.sh: el directorio se resuelve desde la ruta del
# propio script, en vez de un "cd .." que solo funcionaba si se lanzaba desde
# scripts/.
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."

docker build --build-arg ENV=prod --progress=plain -t portable -f scripts/Dockerfile .