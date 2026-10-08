// Asset del motor para Linux: el tar.gz con el binario ELF acestreamengine,
// los módulos en lib/acestreamengine y las wheels.
//
// Va en un fichero con build tag para que el binario de Windows no arrastre los
// 77 MB del motor de Linux, y para que compilar para Windows no exija tener
// descargado a mano el motor de Linux.

//go:build linux

package main

import "embed"

//go:embed assets/acestream-runtime-linux-x86_64.tar.gz
var aceEngineAsset embed.FS
