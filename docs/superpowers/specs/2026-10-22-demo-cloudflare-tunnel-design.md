# Demo pública con Cloudflare Quick Tunnel

## Intención

Publicar temporalmente Belmondo Viajes desde la computadora de desarrollo para
mostrar una demo, sin contratar dominio, VPS ni servicios administrados. La URL
existe solamente mientras Docker y el túnel estén encendidos.

## Alcance

- Una sola URL `https://*.trycloudflare.com` sirve el frontend, la API, los
  WebSockets y los archivos firmados.
- PostgreSQL, Redis y Evolution permanecen accesibles sólo dentro de Docker.
- Un gateway Nginx local escucha exclusivamente en `127.0.0.1:8088` y enruta
  `/api/*` y `/files/*` al backend; el resto va a Next.js.
- El frontend admite un modo de build `same-origin`: HTTP usa rutas relativas y
  WebSocket deriva `wss://` del host abierto en el navegador.
- Dos scripts PowerShell inician y detienen la demo sin modificar `.env` ni
  guardar la URL efímera en el repositorio.

## Acceso y datos

La demo usa el login JWT existente. No se activa `--allowed-mail`: con archivos
locales, Evolution necesita descargar URLs firmadas `/files/*` al enviar un
adjunto y no puede completar el PIN interactivo de Cloudflare. La URL aleatoria
sólo muestra el login; los endpoints de negocio siguen exigiendo JWT y los
archivos conservan firma y vencimiento.

Antes de compartir la URL se desvincula cualquier línea que no deba participar
y se eliminan los datos de prueba que no deban mostrarse, tal como indicó el
propietario del proyecto.

## Ciclo de vida

`scripts/start-demo.ps1` levanta el override de demo, inicia `cloudflared` en un
contenedor, obtiene la URL asignada y recrea el backend con esa URL como
`PUBLIC_BASE_URL`. `scripts/stop-demo.ps1` elimina solamente el contenedor del
túnel, detiene el gateway y devuelve el backend a la configuración normal de
`.env`.

## Fuera de alcance

- URL estable, dominio propio, SLA o disponibilidad permanente.
- Migrar la aplicación a Workers/Pages.
- Publicar Evolution, PostgreSQL o Redis.
- Convertir esta configuración de demo en despliegue de producción.

