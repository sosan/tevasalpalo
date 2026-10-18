"""Hooks de red del motor AceStream (portados del APK v3.2.22.3).

Se carga como sitecustomize del intérprete embebido del motor de Linux: la
app añade este directorio a PYTHONPATH, así que Python lo importa al arrancar
antes de que el motor cree sus sockets. En Windows no aplica: el runtime usa
ace_console.exe con Python 3.8 embebido y sin punto de entrada importable.

Portado de assets/engine/main.py del APK, que parchea en dos sitios:

1. Fallo rápido de DNS para hosts muertos (*.torrentstream.org y
   router.acestream.me apuntan a 54.36.163.2 y no resuelven). Sin esto el
   motor se come el timeout de resolución antes de cada búsqueda y de cada
   anuncio, que es el principal retraso de arranque.

2. Respuestas vacías para VAST y notificaciones, para que una llamada a un
   servidor inalcanzable no bloquee la reproducción 14 s (VAST) o 15 s
   (content_id). Se devuelve un documento válido y vacío, no un error, para
   que el reproductor siga hacia el siguiente paso.

Todo va envuelto en try/except: si algo falla, el motor arranca igual.
"""

import io
import socket
import sys

# Hosts que no resuelven en la práctica. Se comparan en minúsculas.
_DEAD_HOSTS = (
    "torrentstream.org",
    "router.acestream.me",
    "54.36.163.2",
)

# Subcadenas de URL que se cortan con una respuesta vacía.
_DEAD_URLS = (
    "torrentstream.org",
    "vast.php",
    "vast/wrapper",
)
_NOTIF_URL = "android.acestream.net/api/v1/notification"

_installed = False


def _is_dead_host(host):
    if not host:
        return False
    h = str(host).lower()
    return any(k in h for k in _DEAD_HOSTS)


def _is_dead_url(url):
    if not url:
        return False
    u = str(url).lower()
    if any(k in u for k in _DEAD_URLS):
        return True
    if _NOTIF_URL in u:
        return "notif"
    if "router.acestream.me" in u:
        return "dead"
    return False


def _empty_vast():
    buf = io.BytesIO(
        b"<?xml version='1.0' encoding='utf-8'?><VAST version='2.0'></VAST>"
    )
    buf.seek(0)
    return buf


def _empty_notif():
    buf = io.BytesIO(b'{"notifications":[]}')
    buf.seek(0)
    return buf


def _stub_for(url):
    kind = _is_dead_url(url)
    if kind == "notif":
        return _empty_notif()
    if kind:
        return _empty_vast()
    return None


def _patch_dns():
    original = socket.getaddrinfo

    def getaddrinfo(host, *args, **kwargs):
        if _is_dead_host(host):
            raise socket.gaierror(-2, "Name or service not known")
        return original(host, *args, **kwargs)

    socket.getaddrinfo = getaddrinfo


def _patch_urlopen(module, attr):
    original = getattr(module, attr, None)
    if original is None:
        return False

    def urlopen(url, *args, **kwargs):
        stub = _stub_for(getattr(url, "full_url", url))
        if stub is not None:
            return stub
        return original(url, *args, **kwargs)

    setattr(module, attr, urlopen)
    return True


def install():
    global _installed
    if _installed:
        return
    _installed = True

    _patch_dns()

    if _import_urllib("urllib.request"):
        _patch_urlopen("urllib.request", "urlopen")

    # El motor usa su propio wrapper de urlopen con timeout para VAST y
    # notificaciones, así que el parche de urllib por sí solo no basta.
    try:
        from ACEStream.Core.Utilities import timeouturlopen

        _patch_urlopen(timeouturlopen, "urlOpenTimeout")
    except Exception:
        pass

    print("[hooks] bypass de DNS/VAST instalado", flush=True)


def _import_urllib(name):
    try:
        __import__(name)
        return True
    except Exception:
        return False


try:
    install()
except Exception as exc:  # nunca romper el arranque del motor
    print("[hooks] no se pudieron instalar los hooks:", exc, flush=True)
    sys.stdout.flush()