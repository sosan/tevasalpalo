#!/bin/bash

# Build de portable.exe para Windows.
#
# La telemetría va por defecto: si existe un .telemetry.env en la raíz del repo
# (está en .gitignore), se cifra junto con las credenciales de Infisical y se
# incrusta en el binario con el mismo mecanismo que usa CI. Así el .exe sale
# con la telemetría central ya configurada, sin que nadie tenga que exportar
# variables antes de compilar.
#
# Sin .telemetry.env (o sin CLAVE_SECRETOS) el build sigue adelante sin
# secretos: el portable arranca con solo el motor embebido y sin telemetría
# central, que es lo que necesita un fork o una build sin credenciales.

set -euo pipefail

RAIZ="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$RAIZ"

if [ -f ".env.prod" ]; then
    set -a
    # shellcheck disable=SC1091
    source .env.prod
    set +a
else
    echo "❌ Archivo .env.prod no encontrado."
    exit 1
fi

echo "🛠️ Variables de entorno cargadas:"
echo "ENV: $ENV"
echo "GOOS: $GOOS"
echo "GOARCH: $GOARCH"
echo "CGO_ENABLED: $CGO_ENABLED"

VERSION_BUILD=$(date -u "+%Y-%m-%dT%H:%M:%SZ")
echo "Version Build: $VERSION_BUILD"

TELEMETRY_LDFLAGS=""
if [ -f ".telemetry.env" ]; then
    if [ -z "${CLAVE_SECRETOS:-}" ]; then
        echo "⚠️  Hay .telemetry.env pero falta CLAVE_SECRETOS: la telemetría no se incrusta."
        echo "   Para cifrarla: export CLAVE_SECRETOS=\$(openssl rand -hex 32)"
    else
        echo "🔐 Generando el contenedor cifrado de secretos desde .telemetry.env..."
        # El generador imprime el -ldflags entre marcadores; se queda solo con
        # la línea del valor y se descarta el resto (que incluye el recordatorio
        # de que la clave viaja en el binario).
        #
        # GOOS/GOARCH se fuerzan a los del host: .env.prod exporta GOOS=windows
        # para el build, y sin esto el go run compilaría para Windows y no se
        # podría ejecutar aquí para sacar el -ldflags.
        TELEMETRY_LDFLAGS=$(GOOS="$(go env GOHOSTOS)" GOARCH="$(go env GOHOSTARCH)" \
            CGO_ENABLED=0 go run ./cmd/portable --genera-secretos \
            | sed -n 's/^│ \(-X .*\)$/\1/p')
        if [ -n "$TELEMETRY_LDFLAGS" ]; then
            echo "✅ Contenedor cifrado listo."
        else
            echo "⚠️  El generador no devolvió nada; la telemetría no se incrusta."
        fi
    fi
else
    echo "ℹ️  Sin .telemetry.env: la telemetría irá por entorno."
fi

echo "🚀 Compilando para $GOOS/$GOARCH..."
mkdir -p ./build
CGO_ENABLED=$CGO_ENABLED GOOS=$GOOS GOARCH=$GOARCH \
    go build \
    -ldflags="-X 'tevasalpalo/update.VersionBuild=${VERSION_BUILD}' ${TELEMETRY_LDFLAGS}" \
    -o ./build/portable.exe ./cmd/portable

echo "✅ Compilación exitosa: ./build/portable.exe"
if [ -n "$TELEMETRY_LDFLAGS" ]; then
    echo "   (con secretos incrustados: al arrancar verás «🔑 Infisical» y «📊 Telemetría activa»)"
fi