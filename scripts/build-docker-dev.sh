#!/bin/bash
# Compila la imagen Docker de desarrollo.
#
# El directorio se resuelve desde la ruta del propio script. Antes fazia
# "cd .." a ciegas y solo funcionaba si se lanzaba desde scripts/: desde
# streaming/ moria con "lstat scripts: no such file or directory", un error
# que habla de ficheros y no de donde se ejecuto el comando.
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."

docker build --build-arg ENV=dev --progress=plain -t portable -f ./scripts/Dockerfile .