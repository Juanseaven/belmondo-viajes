# Diseño del primer frontend real de la bandeja

## Objetivo

Transformar `/mirror` en una bandeja profesional para operar conversaciones de
WhatsApp de una agencia de viajes. La pantalla debe conservar las funciones que
ya existen, adoptar componentes shadcn reales y evitar controles decorativos o
patrones visuales propios de una plantilla genérica.

El trabajo cubre el shell autenticado y la pantalla de bandeja. No incluye un
rediseño del login ni cambios de backend.

## Principio operativo

El dato más importante, además del contacto y el mensaje, es la línea de
WhatsApp por la que llegó cada conversación. La interfaz mantendrá visible esa
procedencia en la lista, la cabecera del hilo y el panel de detalles.

## Alcance funcional

Se preservan estas funciones existentes:

- listado paginado de conversaciones;
- filtro por línea y bandeja unificada;
- búsqueda de contactos y mensajes;
- lectura paginada del hilo;
- envío optimista de texto y archivos;
- actualización en tiempo real mediante WebSocket;
- conexión de líneas mediante QR;
- desvinculación definitiva de líneas para administradores;
- estados de línea y bloqueo del compositor cuando la línea no está conectada.

El panel derecho incluirá una nota únicamente visual. No se podrá editar ni se
guardará en la base de datos. No habrá etiquetas en esta versión.

Las fotos de perfil no se incorporan al backend en este trabajo. La interfaz
usará `avatarUrl` cuando exista y mostrará iniciales como respaldo.

## Dirección visual

La estética será utilitaria, luminosa y compacta. No se usarán gradientes,
glassmorphism, resplandores, tarjetas decorativas ni radios excesivos.

### Paleta

- Navegación — `#0B1220`
- Fondo de trabajo — `#F5F7FB`
- Superficie — `#FFFFFF`
- Bordes — `#E2E8F0`
- Acción principal — `#146EF5`
- Estado WhatsApp — `#20B15A`

Los estados de error, advertencia y desconexión usarán tokens semánticos de
shadcn, no colores aplicados de manera ornamental.

### Tipografía

- Manrope: títulos, marca y navegación.
- Inter: conversaciones, mensajes, controles y datos.

Ambas se integrarán localmente para que el build no dependa de descargar fuentes
durante el despliegue.

### Elemento distintivo

Una marca de procedencia azul identificará la línea asociada a una conversación.
No será decoración: permitirá reconocer de inmediato qué número está atendiendo
al cliente, incluso dentro de la bandeja unificada.

## Composición

```text
┌──────────────┬───────────────────────────────────────────────────────────┐
│ Navegación   │ Bandeja de entrada        Línea actual    Conectar línea │
│ azul noche   ├──────────────┬──────────────────────┬─────────────────────┤
│              │ Conversacio- │ Conversación activa  │ Datos del contacto  │
│ Bandeja      │ nes + filtros│ y mensajes           │ Línea utilizada     │
│              │              │                      │ Nota visual         │
│              │              │ Compositor           │                     │
├──────────────┴──────────────┴──────────────────────┴─────────────────────┤
```

- Shell: barra lateral fija de aproximadamente 228 px.
- Cabecera: título a la izquierda; selector de línea y acción de conexión a la
  derecha.
- Zona operativa: lista de 320–340 px, hilo flexible y detalles de unos 300 px.
- La página no tendrá scroll general. Lista e hilo tendrán desplazamiento
  independiente y el compositor permanecerá visible.
- En un viewport de 1366×768 deben verse la navegación, la lista, la cabecera del
  hilo y el compositor.
- Cuando no haya ancho suficiente, los detalles se abrirán en un `Sheet` y no se
  comprimirá el hilo.
- En móvil se mostrará primero la lista y después la conversación seleccionada.

## Componentes

Se incorporará una base shadcn dentro del repositorio, con Tailwind y Lucide. Los
componentes no se consumirán como una caja negra: quedarán en `src/components/ui`
para poder adaptarlos manteniendo sus primitivas accesibles.

- `Button`: acciones primarias, envío y controles secundarios.
- `Input` y `Textarea`: búsqueda, confirmación y compositor.
- `Avatar`: foto real o iniciales de respaldo.
- `Badge`: pendientes y estados compactos.
- `DropdownMenu`: selección y administración de líneas.
- `Dialog`: conexión y desvinculación.
- `ScrollArea`: lista e hilo.
- `Sheet`: detalles en ancho intermedio.
- `Tooltip`, `Separator`, `Skeleton` y `Alert`: ayuda y estados de interfaz.

Los controles usarán iconos Lucide. Los emojis actuales dejarán de funcionar
como iconos de interfaz.

## Comportamiento

### Selector de línea

El selector superior ofrecerá “Todas” y las líneas activas. Cada línea mostrará
nombre, número y estado. La gestión administrativa quedará en un menú contextual
con “Desvincular”; “Conectar línea” continuará abriendo el flujo QR existente.

### Bandeja

Cada fila mostrará avatar, nombre, vista previa, hora, pendientes y procedencia.
La selección se indicará con fondo suave y una marca lateral azul.

La búsqueda usará un único campo. Mientras se escribe filtrará los contactos ya
cargados; al presionar Enter ejecutará la búsqueda completa dentro de los
mensajes mediante la API actual. La interfaz explicará esta diferencia mediante
el placeholder y la ayuda accesible, no con un segundo campo permanente.

### Hilo

La cabecera permanecerá fija y mostrará avatar, nombre, teléfono y línea. No se
añadirán botones que no tengan una acción implementada.

Los mensajes conservarán la atribución actual: contacto a la izquierda;
dispositivo o panel a la derecha; nombre del agente cuando corresponda. Los
mensajes provisionales, fallidos, editados, eliminados y con archivos seguirán
siendo distinguibles.

### Compositor

El compositor permanecerá fijo al pie del hilo. Enter enviará y Shift+Enter
insertará una nueva línea. El adjunto, el envío optimista y el error de envío
mantendrán su comportamiento actual. Si la línea no está conectada, un aviso
concreto reemplazará el formulario.

### Detalles

Al abrir una conversación se mostrarán contacto, teléfono, línea utilizada y una
nota de demostración. En escritorio será una columna; en ancho intermedio será un
`Sheet`. Sin conversación seleccionada, no ocupará espacio con contenido falso.

## Flujo de datos

`MirrorScreen` conservará la URL como fuente de verdad para línea, conversación,
mensaje enfocado y búsqueda. Los hooks actuales seguirán controlando paginación y
actualizaciones en tiempo real.

Los componentes visuales recibirán datos y callbacks; no harán solicitudes por
su cuenta, salvo los componentes que ya tienen esa responsabilidad, como la
carga diferida de archivos. El bloque de notas utilizará contenido local marcado
como demostración y no alterará los tipos de la API.

## Estados y errores

- Carga inicial: filas `Skeleton` con dimensiones estables.
- Bandeja vacía: explicación y, para administradores, acceso a conectar una
  línea.
- Sin conversación seleccionada: instrucción breve dentro del hilo.
- Error de bandeja o hilo: `Alert` con acción “Reintentar”.
- Línea desconectada: estado visible y compositor bloqueado.
- Envío pendiente: burbuja provisional.
- Envío fallido: mensaje y estado legibles, sin depender solo del color.
- Foto ausente o fallida: iniciales del contacto.
- Menús y diálogos: foco gestionado, Escape para cerrar y etiquetas accesibles.

## Pruebas y validación

- Pruebas de componentes para selección de línea, búsqueda, avatar de respaldo,
  apertura del panel de detalles y confirmación de desvinculación.
- Conservación de las pruebas actuales del flujo destructivo.
- `npm test`, `npm run typecheck` y `npm run build` como verificación técnica.
- Revisión visual en 1440×900, 1366×768, 1024×768 y un viewport móvil.
- Comprobación de navegación por teclado, foco visible y ausencia de scroll
  horizontal en la página.

## Fuera de alcance

- Persistencia o edición de notas.
- Etiquetas de contactos.
- Sincronización de fotos de perfil con Evolution/WhatsApp.
- Nuevos endpoints de backend.
- Acciones decorativas sin implementación.
- Rediseño del login.
