#!/bin/bash

# Cargar variables de entorno desde .env.prod
if [ -f "../.env.prod" ]; then
    export $(cat ../.env.prod | xargs)
else
    echo "❌ Archivo .env.prod no encontrado."
    exit 1
fi

# Verificar que las variables de entorno se cargaron correctamente
echo "🛠️ Variables de entorno cargadas:"
echo "ENV: $ENV"
echo "GOOS: $GOOS"
echo "GOARCH: $GOARCH"
echo "CGO_ENABLED: $CGO_ENABLED"

# Ejecutar el comando de compilación
echo "🚀 Compilando para $GOOS/$GOARCH..."
# go build -o ../build/portable.exe ../.
GOOS=$GOOS GOARCH=$GOARCH CGO_ENABLED=$CGO_ENABLED go build -o ../build/portable.exe ../.

# Verificar si la compilación fue exitosa
if [ $? -eq 0 ]; then
    echo "✅ Compilación exitosa: ../build/portable.exe"
else
    echo "❌ Error durante la compilación."
    exit 1
fi
