// Entry point del portable: la lógica de arranque vive en el paquete app
// (tevasalpalo/internal/app), incluida su init() de autoupdate, que se
// ejecuta igualmente al importar el paquete.
package main

import (
	"tevasalpalo/internal/app"
)

func main() {
	app.Main()
}
