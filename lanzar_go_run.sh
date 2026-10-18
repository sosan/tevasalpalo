#!/usr/bin/env bash
# Lanza el proyecto igual que la configuración "Launch" de VS Code (mode: debug).
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$DIR"

VERSION_BUILD="${VERSION_BUILD:-$(date -u "+%Y-%m-%dT%H:%M:%SZ")}"

SECRETOS='v1:qfRnLe1mB9G/gy359IIf84ZCXKrHKFc39flvHCCL3GFR5mLgEOvMc26i/aQBCzr8j9G3xvb3UF1QnmYzSeB1Z2/0KzmN7AdHAWX+a4T2LGBZVt2pl9bePuph+AZxZylHIeSVNLM7lK/AFhuhAoz9ZNJ2PstNjXqbIeegqC2V9rI4NIbnnhUxKBZASxWc3W5uSAmnGkSs70Thal0oASvgteCC+QNlLL8an+oOWrjLpu6UrW+J3oTu7bJBKfCJKWnkTMDDS7Ik2ZwsE31ERhhpny6enBjF3ynAV5PVR1jPjXQizXRnwfQhK1ooA9lL'
CLAVE='alddaq11235Qe211QWEQWRRWEWER23773847583AMFFE__334WERWERQMMGHFGH42234--eejyRRWEGGDyewwerw'

LDFLAGS="-X tevasalpalo/update.VersionBuild=${VERSION_BUILD} -X tevasalpalo/internal/app.embebidoSecretos=${SECRETOS} -X tevasalpalo/internal/app.claveSecretos=${CLAVE}"

# Carga el envFile igual que VS Code
set -a
source "$DIR/.env.dev"
set +a

go run -ldflags="$LDFLAGS" ./cmd/portable
