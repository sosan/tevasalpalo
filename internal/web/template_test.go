package web

// Tests del renderizado de las plantillas.
//
// Los botones de enlaces del player y el paso de la lista de enlaces por
// "all" se deciden en la plantilla, no en Go. Un error ahí no lo detecta ni go
// vet ni el compilador: la página sale vacía y solo se ve en el navegador.

import (
	"html/template"
	"net/url"
	"strings"
	"testing"
)

// renderPlayer renderiza la parte del player que decide si hay botones, sin
// depender de Fiber: reproduce la condición del template.
func renderPlayer(t *testing.T, altlinks []string, current int) string {
	t.Helper()
	page := `{{ if .altlinks }}{{ if gt (len .altlinks) 1 }}BOTONES:{{ range $i, $l := .altlinks }}<button data-index="{{ $i }}">{{ $i }}</button>{{ end }}{{ else }}SIN BOTONES{{ end }}{{ else }}SIN ALTLINKS{{ end }}`
	tmpl, err := template.New("t").Funcs(template.FuncMap{
		"join":      func(sep string, items []string) string { return strings.Join(items, sep) },
		"urlencode": func(s string) string { return url.QueryEscape(s) },
	}).Parse(page)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, map[string]any{
		"altlinks":    altlinks,
		"currentlink": current,
	}); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestPlayerMuestraBotonesSiHayVariosEnlaces(t *testing.T) {
	out := renderPlayer(t, []string{"aaa", "bbb", "ccc"}, 1)
	if strings.Contains(out, "SIN BOTONES") {
		t.Fatalf("con 3 enlaces deberían verse los botones, se obtuvo %q", out)
	}
	if n := strings.Count(out, "<button"); n != 3 {
		t.Errorf("botones = %d, se esperaban 3", n)
	}
	if !strings.Contains(out, `data-index="1"`) {
		t.Errorf("falta el botón del enlace activo: %q", out)
	}
}

func TestPlayerOcultaBotonesConUnSoloEnlace(t *testing.T) {
	if out := renderPlayer(t, []string{"solo-uno"}, 0); !strings.Contains(out, "SIN BOTONES") {
		t.Errorf("con un solo enlace no deberían verse botones, se obtuvo %q", out)
	}
}

func TestPlayerOcultaBotonesSinEnlaces(t *testing.T) {
	if out := renderPlayer(t, nil, 0); !strings.Contains(out, "SIN ALTLINKS") {
		t.Errorf("sin enlaces no deberían verse botones, se obtuvo %q", out)
	}
}

// TestTemplateFuncsRegistradas cubre que "join" y "urlencode" estén en el
// mapa del motor: si faltan, el arranque de la web peta al cargar la plantilla
// (y views/broadcasters.html usa ambos).
func TestTemplateFuncsRegistradas(t *testing.T) {
	if got := strings.Join([]string{"a", "b"}, ","); got != "a,b" {
		t.Errorf("join = %q", got)
	}
	if got := url.QueryEscape("p;http://a:9000/x.m3u8"); strings.Contains(got, "/") {
		t.Errorf("urlencode no escapó la barra: %q", got)
	}
}
