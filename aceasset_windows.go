// Asset del motor para Windows: el runtime empaquetado en
// assets/acestream-runtime-windows.zip (ace_console.exe + python38 + .pyd).
//
// Va en un fichero con build tag para que el binario de Linux no arrastre los
// 74 MB del motor de Windows, y para que compilar para Windows no exija tener
// descargado a mano el motor de Linux.

//go:build windows

package main

import "embed"

//go:embed assets/acestream-runtime-windows.zip
var aceEngineAsset embed.FS
