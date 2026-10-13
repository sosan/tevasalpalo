#!/bin/bash
# Sella portable.exe con el certificado de certs/.
#
# Las rutas se resuelven desde la de este script y no desde el directorio en el
# que se lance. Antes "-in portable.exe" y "-pkcs12 ../certs/..." dependían del
# cwd, que solo casaba si se ejecutaba desde build/.
set -euo pipefail

RAIZ="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"

# build-windows.sh deja el binario en build/, que es el mismo fichero que
# "./portable.exe" cuando se ejecutaba desde ahí.
INPUT="${1:-$RAIZ/build/portable.exe}"
OUTPUT="${2:-$RAIZ/build/portable.firmado.exe}"
PFX="$RAIZ/certs/certificado.pfx"
PASS="1234"

if [ ! -f "$INPUT" ]; then
  echo "❌ No existe el ejecutable a firmar: $INPUT"
  echo "   Compílalo antes con scripts/build-windows.sh"
  exit 1
fi
if [ ! -f "$PFX" ]; then
  echo "❌ No existe el certificado: $PFX"
  exit 1
fi

TIMESTAMP_SERVERS=(
  "http://timestamp.sectigo.com"
  "http://timestamp.digicert.com"
  "http://timestamp.comodoca.com"
)

for ts in "${TIMESTAMP_SERVERS[@]}"; do
  echo "🔧 Intentando con: $ts"
  if osslsigncode sign \
    -pkcs12 "$PFX" \
    -pass "$PASS" \
    -n "AceStream Portable" \
    -t "$ts" \
    -in "$INPUT" \
    -out "$OUTPUT"; then
    echo "✅ Firmado exitosamente con $ts"
    osslsigncode verify "$OUTPUT"
    exit 0
  else
    echo "❌ Falló con $ts"
  fi
done

echo "❌ Todos los servidores de timestamp fallaron"
exit 1