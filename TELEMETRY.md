# Telemetría y logs centralizados

Los logs del servidor y los eventos del player se pueden enviar a un punto
central (Grafana Cloud Loki vía OTLP) para diagnosticar errores a distancia.
Todo es opcional: si no hay nada configurado, solo se escribe en stdout.

## Variables de entorno

| Variable | Por defecto | Para qué |
|---|---|---|
| `TELEMETRY_ENABLED` | `false` | Activa el envío al central. |
| `TELEMETRY_OTLP_URL` | (vacío) | Endpoint del gateway regional: `https://otlp-gateway-prod-eu-west-0.grafana.net/otlp`. Con o sin `/v1/logs`. |
| `TELEMETRY_USER` | (vacío) | Basic auth: el **Instance ID** de la consola (p. ej. `251374`). |
| `TELEMETRY_PASSWORD` | (vacío) | Token de API (`glc_...`). |
| `TELEMETRY_LOG_DIR` | (vacío) | Carpeta para el fichero de log local. Si vacía, no se escribe fichero. |
| `TELEMETRY_HASH_SALT` | (vacío) | Salt para hashear IPs. **Póntelo siempre**: sin salt el hash de IP es trivial de revertir. |
| `TELEMETRY_LOG_MAX_BYTES` | `8388608` | Tamaño máximo por fichero antes de rotar. |
| `TELEMETRY_LOG_MAX_FILES` | `5` | Número de ficheros rotados que se conservan. |
| `TELEMETRY_FLUSH_MS` | `5000` | Cada cuánto se envía el lote al central. |
| `TELEMETRY_MAX_BATCH` | `200` | Máximo de registros por envío. |
| `TELEMETRY_BUFFER` | `2000` | Registros que se guardan en memoria y en la cola de envío. |
| `TELEMETRY_EXPORT_LOGS` | `true` | Copia también las líneas de `log.Printf` al central (ver "Qué se envía"). |
| `TELEMETRY_API_TOKEN` | (vacío) | Si se define, `/api/logs` exige este token. Si no, solo se sirve desde loopback. |
| `API_LOG_LINES` | `200` | Líneas por defecto en `/api/logs` (tope 2000). |

Las credenciales se leen solo del entorno y **nunca se envían al navegador**.
El player postea a `/api/telemetry` de esta misma app y el servidor reenvía.

## De dónde salen las credenciales

Hay tres fuentes, en este orden de prioridad:

1. **El entorno** (`.telemetry.env`, PowerShell, Docker). Gana siempre.
2. **Infisical**, si está configurado (ver más abajo). Solo rellena lo que esté
   vacío.
3. Nada: la app arranca igual, solo con fichero local y memoria.

La prioridad importa: si apuntas a mano a otro Loki, una rotación en Infisical
no te lo pisa por debajo.

### Envío directo a Grafana Cloud (lo normal)

La app habla OTLP con el gateway regional mediante el SDK oficial de
OpenTelemetry. Es lo que monta la consola de Grafana Cloud:

```sh
export TELEMETRY_ENABLED=1
export TELEMETRY_OTLP_URL="https://otlp-gateway-prod-eu-west-0.grafana.net/otlp"
export TELEMETRY_USER="251374"      # Instance ID
export TELEMETRY_PASSWORD="glc_..."
```

El endpoint se puede poner con o sin `/v1/logs`: la app lo normaliza. El SDK
parte la URL en host y ruta y **no añade** la ruta de la señal, así que pasarla
tal cual daría un 404 silencioso; por eso se normaliza aquí.

Dos cosas que confunden:

- El usuario es el **Instance ID** (`251374`), no el campo `"o"` que va dentro
  del token (`540733` en este caso). Con el segundo el gateway responde 401.
- El gateway es **multi-zona**: los registros no van todos al mismo backend, así
  que el orden entre consultas puede no ser exacto. Para diagnostics está bien;
  no sirve para afirmar sobre el orden.

### Usar el collector sin cambiar la app

Si defines las variables estándar de OpenTelemetry y **no** defines
`TELEMETRY_OTLP_URL`, el SDK las lee por su cuenta. Eso permite poner un
collector por delante sin tocar la app:

```sh
export OTEL_EXPORTER_OTLP_PROTOCOL="http/protobuf"
export OTEL_EXPORTER_OTLP_ENDPOINT="https://otlp-gateway-prod-eu-west-0.grafana.net/otlp"
export OTEL_EXPORTER_OTLP_HEADERS="Authorization=Basic <base64 de instancia:token>"
```

### Por qué el token no va dentro del binario

El workflow publica `portable.exe` en un release público de GitHub. Cualquier
cadena metida con `-ldflags -X` sale con `strings portable.exe`. Vale igual para
el token de Grafana, el de Infisical o el hash salt.

Lo que sí se puede incrustar es lo que no es secreto (la URL del endpoint), pero
tampoco hace falta: la app es la fuente de verdad de su configuración.

### Por qué Infisical y no el token dentro del binario

El workflow publica `portable.exe` en un release público de GitHub
(`buidandtest.yaml`). Cualquier cadena metida con `-ldflags -X` acaba
legible con `strings portable.exe`. Eso aplica igual al token de Grafana y al
de Infisical: **ninguno de los dos va dentro del binario**.

Lo que sí aporta Infisical es la rotación central: cambias el token de Grafana
en la consola y todas las instalaciones lo recogen en el siguiente arranque,
sin publicar un binario nuevo.

### Configurar Infisical

Crea en la consola un **service token** con permiso `read` acotado a la ruta
donde tengas los secretos (por ejemplo `/streaming`), en los entornos que
quieras. Anota el **project ID** y el slug del entorno: no van en el token, hay
que configurarlos aparte.

Dentro de la carpeta, añade al menos:

| Clave | Contenido |
|---|---|
| `TELEMETRY_OTLP_URL` | `https://otlp-gateway-prod-eu-west-0.grafana.net/otlp/v1/logs` |
| `TELEMETRY_USER` | ID de instancia de Loki (`251374`) |
| `TELEMETRY_PASSWORD` | El token `glc_...` |

Después, en cada instalación:

```sh
export INFISICAL_TOKEN="st...."      # tu token de Infisical
export INFISICAL_PROJECT_ID="d818000d-1ce2-4bc5-aaa5-77907d1f3927"
export INFISICAL_ENV="prod"
export INFISICAL_PATH="/streaming"
export TELEMETRY_ENABLED=1
./portable
```

| Variable | Por defecto | Para qué |
|---|---|---|
| `INFISICAL_ENABLED` | `1` | Ponlo a `0` para no consultar Infisical ni al arrancar. |
| `INFISICAL_TOKEN` | (vacío) | Token de lectura. Sin él, no se consulta. |
| `INFISICAL_PROJECT_ID` | (vacío) | ID del proyecto (Project Settings → General). |
| `INFISICAL_ENV` | (vacío) | Slug del entorno: `dev`, `prod`… |
| `INFISICAL_PATH` | `/` | Carpeta donde están los secretos. |
| `INFISICAL_URL` | `https://app.infisical.com` | Cambiar solo si usas self-hosted. |
| `INFISICAL_TIMEOUT_MS` | `5000` | Timeout de la petición. |

Al arrancar verás `🔑 Infisical: N secretos de /streaming (prod) aplicados: ...`
y, si algo falla, un `⚠️` que dice qué corregir. Si la carpeta no existe, el
error lo dice explícitamente.

## Configurar Grafana Cloud (Loki)

Lo más fácil es copiar el ejemplo y rellenarlo:

```sh
cp .telemetry.env.example .telemetry.env
$EDITOR .telemetry.env
```

`.telemetry.env` está en `.gitignore` para que el token no acabe en el repo.
Después se carga en el entorno (en Windows, PowerShell):

```powershell
Get-Content .telemetry.env | ForEach-Object {
    if ($_ -match '^([^#][^=]*)=(.*)$') {
        Set-Item -Path "env:$($matches[1])" -Value $matches[2]
    }
}
.\portable.exe
```

Equivalente a mano en cualquier shell:

```sh
export TELEMETRY_ENABLED=1
export TELEMETRY_OTLP_URL="https://otlp-gateway-prod-eu-west-0.grafana.net/otlp/v1/logs"
export TELEMETRY_USER="251374"
export TELEMETRY_PASSWORD="glc_..."
export TELEMETRY_HASH_SALT="una-cadena-aleatoria-larga-y-secreta"
export TELEMETRY_LOG_DIR="$HOME/streaming-logs"
./portable
```

### Ojo con el usuario de Basic auth

El token `glc_` lleva dentro un JSON con `"o":"540733"`, pero **ese no es el
usuario**. El gateway espera el Instance ID. Con el valor equivocado:

```
401 {"status":"error","error":"authentication error: invalid authentication credentials"}
```

El Instance ID lo pone la propia consola al copiar el bloque de "Environment
Variables", en `OTEL_EXPORTER_OTLP_HEADERS`: es el número delante de los dos
puntos, codificado en base64. Para comprobarlo:

```sh
echo 'Basic MjUxMzc0...' | cut -d' ' -f2 | base64 -d
# 251374:glc_...
```

Al arrancar verás `📊 Telemetría activa (fichero local + central)`.
Si ves `(fichero local)`, el central no está configurado o falló; el detalle
está en `GET /api/logs` → `exporter.last_error`.

En Grafana, los eventos llegan como `service_name="streaming"`. Filtra por
`client_id` para separar instalaciones y por `severity_text` (`ERROR`,
`WARN`, `INFO`) para ver sólo fallos.

## Qué se envía

**Del servidor.** Dos cosas distintas:

1. **Líneas de log** (`event=server_log`): todo lo que ya escribía el
   programa con `log.Printf` va también al central, con los campos `nivel` e
   `informe`. No hay que tocar ninguna llamada existente: es un writer más en
   el mismo `MultiWriter`. Para que no se inunden, una línea idéntica y
   consecutiva se colapsa: se envía la primera y luego en potencias de dos
   (2, 4, 8...), cada una con `repeticiones=N`.
2. **Eventos estructurados** con `Record(...)`, que llevan su propio nombre:
   `player_media_attached`, `player_manifest_parsed`, `player_play`,
   `player_switch_link`, `player_autoplay_blocked`, `player_hls_error`.

**Del navegador** (`views/player.html`): errores HLS (fatal y no fatal,
agrupados cada 10), tiempo hasta el primer manifest, autoplay y cambio de
enlace. El player agrupa en lotes de 20 y postea a `/api/telemetry`:

```json
{"events": [{"event": "player_hls_error", "fields": {...}},
            {"event": "player_play", "fields": {...}}]}
```

Cada evento conserva su nombre y sus campos. El servidor acepta también el
formato antiguo de evento suelto (`{"event":..., "fields":...}`), por si
quedara algún player cacheado. Los lotes se limitan a 50 eventos y 64 KB por
petición.

## Qué consultar en Grafana

| Consulta | Para qué |
|---|---|
| `{service_name="streaming"} \|= "levelLoadTimeOut"` | Errores de carga del manifest. |
| `{service_name="streaming"} \| json \| server_log != ""` | Solo líneas de log del servidor. |
| `{service_name="streaming"} \| json \| fatal="true"` | Solo los errores HLS que pararon la reproducción. |
| `sum by (client_id) (count_over_time(...[5m]))` | Errores por instalación. |

Los atributos llegan con su tipo (`boolValue`, `intValue`, `stringValue`), así
que se pueden agregar y comparar sin parsear texto.

## Privacidad

- La IP del cliente **se hashea** con `TELEMETRY_HASH_SALT` antes de enviarse
  (sal estable → puedes correlacionar una sesión sin exponer la real).
- El resto va **literal**: rutas `/ace/...`, errores y métricas. Es
  deliberado, porque son justamente lo que sirve para depurar. Los tokens de
  las URLs del manifest sí viajan; si algún díaPrefieres ocultarlos, se añade
  aquí.

## Diagnóstico local

El servidor escucha en `0.0.0.0`, pero `/api/logs` **solo responde a
loopback** (o con `TELEMETRY_API_TOKEN`). El log lleva URLs y hashes
literales y no cualquiera de la red debería leerlos.

```sh
curl -s http://127.0.0.1:3000/api/logs | jq          # últimas líneas + estado
curl -s http://127.0.0.1:3000/api/logs?lines=500    # más histórico (tope 2000)

# Desde otro equipo, con token:
curl -s -H "X-Telemetry-Token: $TELEMETRY_API_TOKEN" \
     http://192.168.1.50:3000/api/logs | jq
```

`/api/logs` devuelve:
- `lines`: últimas líneas del log en memoria.
- `config`: si está habilitada, si hay exportador, y cuántos hay en buffer.
- `exporter`: lote en cola, enviados, descartados, y `last_error` (por ejemplo
  `401 (credenciales ... incorrectas)`).

## Comportamiento ante fallos

- Si el central no responde, los lotes se acumulan hasta el tope de `TELEMETRY_BUFFER`
  y se descartan los más antiguos; nunca se bloquea el player.
- Los reintentos usan backoff exponencial (tope 2 min).
- Al cerrar la app (Ctrl+C o autoupdate) se intenta un último envío antes de
  morir, para que no se pierda el evento que explica el fallo.
- Si no hay `TELEMETRY_LOG_DIR`, no se toca disco (solo stdout + memoria).
- El SDK de OpenTelemetry se encarga del payload, la compresión y los reintentos.
  Antes se serializaba a mano y se confundía el `body` (que es un AnyValue
  pelado) con los atributos (`{key, value}`): Loki devolvía 400 y, peor, un body
  mal interpretado se descartaba sin dejar rastro.

## Verificar que funciona

Hay dos pruebas, y las dos se saltan solas si no hace falta nada:

```sh
# 1. El camino completo contra un central simulado (sin red, sin credenciales)
go test -run TestLogPrintfLlegaAlCentral -v

# 2. Contra el gateway real de Grafana, solo si defines las variables
TELEMETRY_OTLP_URL="https://otlp-gateway-prod-eu-west-0.grafana.net/otlp/v1/logs" \
TELEMETRY_USER="251374" \
TELEMETRY_PASSWORD="glc_..." \
go test -run TestGatewayRealDeGrafana -v
```

Si la primera pasa, un `log.Printf` cualquiera acaba en el central con el
schema correcto. La segunda confirma que el token y el usuario son buenos.

## Receptor propio (opcional, no implementado aún)

Para installations sin Grafana Cloud (o como red de seguridad offline), la
misma app ya escribe JSONL rotado en `TELEMETRY_LOG_DIR`. Levantar un
receptor que recoja esos ficheros y los sirva por HTTP es un añadido
independiente; el formato ya es JSON Lines para hacerlo trivial.
## Secretos cifrados dentro del binario

Para que el `portable.exe` salga con la configuración ya puesta (sin que el
usuario tenga que exportar variables), el build puede incrustar un contenedor
cifrado con AES-256-GCM.

**Qué protege, y es real:** el token no está en claro en el binario.
`strings portable.exe` no lo encuentra, ni aparece en un grep del repo, ni en
los logs de CI, ni en un volcado de memoria. Además el contenedor está
autenticado: si alguien lo edita, el descifrado falla en vez de devolver basura.

**Qué no protege:** la clave viaja en el mismo binario, así que quien invierta
el descifrado la obtiene. Esto evita la filtración accidental, no a un atacante
con tiempo. Para una instalación seria, el token debe ir en el entorno.

### Generar el contenedor

```sh
CLAVE_SECRETOS=una-clave-larga-y-aleatoria \
INFISICAL_TOKEN=st.... \
INFISICAL_PROJECT_ID=... \
INFISICAL_ENV=prod \
INFISICAL_PATH=/streaming \
./portable --genera-secretos
```

Imprime el valor del secret ya sin envoltorio, delimitado para copiarlo tal cual
(lo importante es que no lleva `-ldflags "` ni comillas colgando):

```
1) Valor del secret de GitHub Actions (TELEMETRY_LDFLAGS):

┌────────────────────────────────────────────────────────────────────────────┐
│ -X 'tevasalpalo/internal/app.embebidoSecretos=v1:BASE64' -X 'tevasalpalo/internal/app.claveSecretos=CLAVE'
└────────────────────────────────────────────────────────────────────────────┘
```

Si detecta que la clave o el token son valores de relleno, avisa antes de que
acabes con un binario que compila pero no autentica.

### Compilar con secretos

En `.github/workflows/buidandtest.yaml`, el paso "Build binary" pasa a tener:

```sh
-ldflags="-X 'tevasalpalo/update.VersionBuild=${VERSION_BUILD}' ${TELEMETRY_LDFLAGS}"
```

donde `TELEMETRY_LDFLAGS` es un secret de Actions con la línea que imprime el
generador. Nota: **no lo pongas en `.env.prod`**, que el job también carga y que
acaba en los logs del build.

### Al arrancar

El contenedor se descifra antes de nada, se aplica al entorno **sin pisar lo
que ya esté definido**, y la clave y el texto plano se borran de memoria. Si el
contenedor no se puede abrir se avisa y la app arranca igual, sin telemetría
central: perder los logs no puede impedir ver el vídeo.

Prioridad final: **entorno > contenedor cifrado > nada**.
