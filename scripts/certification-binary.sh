#!/bin/bash

INPUT="portable.exe"
OUTPUT="portable.firmado.exe"
PFX="../certs/certificado.pfx"
PASS="1234"

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