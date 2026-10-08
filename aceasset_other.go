// Para cualquier SO que no sea Windows ni Linux no hay motor empaquetado: el
// programa arranca y usa ACESTREAM_API contra un motor remoto.

//go:build !windows && !linux

package main

import "embed"

var aceEngineAsset embed.FS
