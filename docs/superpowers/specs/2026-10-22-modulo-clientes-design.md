# Diseño de la pantalla de Clientes

Maqueta aprobada (abrir en el navegador, ancho 1440 px):
[Clientes: lista, página del cliente, viaje sin WhatsApp y nuevo cliente](maquetas/2026-10-22-clientes/clientes.html).
La maqueta manda en lo visual; las reglas del rediseño
(`2026-10-21-rediseno-ui-design.md`) valen igual acá.

## Por qué

Hoy un cliente solo existe como algo que cuelga de un chat: un viaje se crea
desde una conversación de WhatsApp (`createTrip` exige `contactId`) y el
cliente nace con ella. Eso deja dos huecos:

1. **No hay dónde ver a un cliente** con todos sus viajes y sus datos.
2. **Quien compra por teléfono o en la oficina no puede tener un viaje.** No
   tiene chat, y sin chat no hay viaje.

## Qué es un cliente

**Quien compra el viaje**, la tabla `customers` que ya existe. No cambia lo
que es: nombre, teléfono, mail, documento, cliente desde, y los contactos de
WhatsApp que tenga vinculados (`customer_contacts`, que puede ser ninguno).

Los pasajeros **siguen siendo de cada viaje**. Guardar a cada persona una sola
vez y reusarla entre viajes se discutió y quedó afuera: toca el motor de
requisitos y los controles, y su valor depende de que los clientes vuelvan a
comprar, algo que todavía no se sabe. En su lugar, **un viaje nuevo puede
traer copias de los pasajeros de los viajes anteriores del mismo cliente**
(ver «Traer pasajeros»). Si la agencia confirma que los clientes repiten y la
copia se queda corta, el paso a «personas» queda abierto.

**Un cliente «sin WhatsApp»** es uno sin ningún contacto vinculado que tenga una
conversación individual (no de grupo) con alguna línea. Es la única definición
que se usa en todo el documento, en el servidor y en la pantalla: decide a
dónde se abre un viaje, si existe «Marcar hecho» y qué dice la columna
Contacto.

No alcanza con mirar `customer_contacts`: hay contactos vinculados sin chat
(el alias que deja la fusión de LID de la migración 0007 del espejo, y los
contactos que llegaron solo por el directorio). Un cliente así no tiene dónde
recibir el mensaje que prueba un control, y se trata como sin WhatsApp. Es la
misma condición que ya usan Pendientes y Viajes para el «Sin chat»
(`conversationId` vacío).

## Objetivos

- Una pantalla **Clientes** con la lista de todos los clientes y una página
  por cliente con sus datos, sus viajes, sus chats y con quién viaja.
- Crear un cliente sin chat y armarle un viaje de cero con todo lo que hoy
  tiene un viaje (recorrido, fechas, pasajeros, pago, requisitos, controles).
- Que un viaje de un cliente sin WhatsApp se pueda trabajar entero, incluidos
  los controles que hoy solo se cierran mandando un mensaje.
- Vincular un chat a un cliente cargado a mano cuando después escribe.
- Traer pasajeros de viajes anteriores del cliente.

## Fuera de alcance

- Guardar a cada persona una vez (ver arriba).
- **Unir dos clientes** que resultaron ser el mismo. Vincular solo acepta
  contactos que todavía no son de ningún cliente (ver «Vincular un chat»).
- Desvincular un contacto, borrar un cliente.
- Reportes por cliente (cuánto compró, frecuencia).
- Modo oscuro.

## Dónde vive

Una entrada nueva en la barra lateral, **Clientes**, después de Viajes, con el
ícono de personas de la maqueta. Rutas:

- `/clientes`: la lista. Búsqueda, filtro y orden en la URL, como Viajes.
- `/clientes/<id>`: la página del cliente. `?viaje=<tripId>` abre el panel del
  viaje sobre la página; `?nuevo-viaje=1` abre el formulario de nuevo viaje.

Es parte del módulo `operaciones`: la tabla `customers` ya es suya y la
pantalla la ve quien ve Viajes. Hoy el frontend tiene un `ModuleDef` por módulo
con una sola ruta; cómo se suma una segunda entrada de menú para el mismo
módulo del backend queda al constructor, con una condición: Clientes aparece si
y solo si `/api/me` autoriza `operaciones`.

## La lista (`/clientes`)

Ver la primera pantalla de la maqueta. El mismo esquema que «Todos los viajes».

- Encabezado: título «Clientes» y el botón «Nuevo cliente».
- **Buscar** en un solo campo: nombre del cliente, teléfono (por dígitos: «4788
  2210» encuentra «11 4788-2210» y «+5491147882210»), mail, número de
  documento, y nombre o apellido de cualquier pasajero de sus viajes. Sin
  distinguir mayúsculas ni tildes, con el mismo `fold` de `lists.go`. El
  teléfono busca también en los contactos vinculados.
- **Chips**, con cuántos clientes hay en cada uno con la búsqueda aplicada:
  Todos (por defecto) · Con viaje abierto · Sin viajes · Sin WhatsApp. Son
  excluyentes, como los estados en Viajes.
- Debajo: «7 clientes · ordenados por el último movimiento».
- **Columnas:**

| Columna | Arriba | Abajo, en gris |
|---|---|---|
| Cliente | avatar e iniciales, nombre | «cliente desde mar 2024» |
| Contacto | con WhatsApp: globo verde y el teléfono del contacto más reciente; sin WhatsApp: teléfono (o mail si no hay teléfono) con ícono gris | «N chats» si tiene más de uno; «sin WhatsApp» si no tiene |
| Viajes | cuántos | «último: <destino>» si tiene más de uno |
| Viaje abierto | «#12 Japón · sale lun 02/11» del más reciente sin cerrar | su estado como etiqueta de color |

  Sin viajes: «Sin viajes» en gris y «—» en Viaje abierto. Sin teléfono ni mail
  ni WhatsApp: «Sin contacto» en gris.
- **Orden:** último movimiento primero. El último movimiento es el más
  reciente entre: el alta y la última edición del cliente; el alta y la última
  edición de cualquiera de sus viajes; la última escritura en el pago de esos
  viajes; el alta y la última edición de sus pasajeros; y el último control
  cerrado (`control_evidence.closed_at`). Los mensajes de WhatsApp no cuentan:
  ese orden ya lo da la bandeja. Cómo se calcula (al leer o manteniendo una
  columna) queda al constructor. También se puede ordenar por nombre.
- **Paginación** por cursor de a 50 con carga al llegar al final, como Viajes.
- Toda la fila abre la página del cliente.
- Vacío con búsqueda: «Ningún cliente coincide» y «Limpiar búsqueda». Sin
  ningún cliente: «Todavía no hay clientes» con «Nuevo cliente».
- Avatar: la foto del contacto más reciente si tiene WhatsApp; si no,
  iniciales con el color sacado del nombre, como en el resto de la app.

## La página del cliente (`/clientes/<id>`)

Ver la segunda pantalla de la maqueta.

- **Encabezado:** «‹ Clientes» (vuelve a la lista con la búsqueda y el filtro
  como estaban), avatar grande, nombre, y a la derecha «Editar» y «Nuevo
  viaje».
- **Datos:** teléfono, mail, documento y cliente desde, en una franja con
  íconos. Lo que falta dice qué falta («Sin mail», en gris).
- **Editar** abre la misma ventana que «Nuevo cliente» con los datos cargados,
  y guarda con el `PATCH` de cliente que ya existe (que ya recalcula los
  derivados de sus viajes). Suma «Cliente desde» como campo de fecha.
- **Viajes:** todos, abiertos primero y después cerrados, cada grupo del más
  reciente al más viejo. Columnas como en Viajes: #, Destino con su estado,
  Salida con la vuelta, Total con el estado de la seña, Lo que sigue. Vacío:
  «Todavía no tiene viajes» con «Nuevo viaje».
- **Al tocar un viaje:**
  - cliente con WhatsApp → va al chat, igual que desde Viajes
    (`/mirror?c=<conversación más reciente>&viaje=<id>`);
  - cliente sin WhatsApp → abre el **panel del viaje** desde la derecha sobre
    la página (ver abajo).
- **WhatsApp** (columna derecha): cada contacto vinculado con su nombre,
  teléfono y la línea de su conversación más reciente, con «Abrir chat». Sin
  contactos: «No tiene chats», la frase «Si escribe desde <teléfono>, la
  bandeja va a ofrecer vincularlo» (sin la frase si no hay teléfono) y
  «Vincular un chat».
- **Con quién viaja** (columna derecha): las personas que fueron pasajeras en
  sus viajes, sin repetir (ver «Quién es la misma persona»), con cuántos viajes
  y el documento con que viajó la última vez. Solo lectura. No aparece si
  ningún viaje tiene pasajeros.

## El panel del viaje sin chat

Ver la tercera pantalla de la maqueta, izquierda.

Es el mismo `TripPanel` de la bandeja, con las mismas pestañas y acciones,
dentro de un panel que entra desde la derecha (el `sheet` de `components/ui`)
sobre la página. Se cierra con la ✕, con Escape o tocando afuera; cerrarlo
saca `?viaje=` de la URL. El selector de viajes del encabezado del panel
cambia entre los viajes de ese cliente.

Hoy `TripSidebar` arma el panel a partir de un contacto (`useTrips` por
contacto). Hace falta lo mismo a partir de un cliente: leer sus viajes y el
detalle del elegido, recargar ante los eventos de `operaciones`. Cómo se
reparte entre `TripSidebar` y la pieza nueva queda al constructor.

**El mismo panel se usa en Viajes** para los viajes de clientes sin WhatsApp:
en Pendientes y en Todos los viajes, donde hoy hay un «Sin chat» desactivado,
el botón pasa a decir «Abrir viaje» y abre este panel sobre la pantalla de
Viajes.

### «Marcar hecho»

Los controles con `closing = 'accion'` hoy solo se cierran con un mensaje como
prueba (`ErrEvidenciaRequerida`). Un cliente sin chat no puede recibirlo, y
esos controles quedarían abiertos para siempre.

Regla nueva: **si el cliente del viaje no tiene WhatsApp** (según la definición
de arriba), un control por acción se puede cerrar sin mensaje. El botón dice «Marcar hecho»
en vez del `actionLabel`, y la evidencia queda como en los manuales: quién y
cuándo, sin `message_id`. Los controles por acción de un cliente con WhatsApp
siguen exigiendo el mensaje.

- La regla se aplica en el servidor (`CloseControl`), no solo en el botón.
- El aviso de «Requisitos migratorios» (`control-visa`) guarda igual qué
  requisitos estaban en falta (`notified`), así un requisito nuevo lo reabre
  como hoy.
- En «Lo que sigue» del Resumen, debajo del control, va la nota de la
  maqueta: «<Nombre> no tiene WhatsApp: lo que en otros viajes se cierra al
  mandar el mensaje, acá se marca a mano y queda quién y cuándo.» Solo en el
  Resumen, no en cada renglón de Controles.
- En Hechos, el control cerrado así se lee igual que uno manual.
- Si después se vincula un chat, lo ya cerrado queda cerrado; lo abierto pasa
  a pedir el mensaje.

Las acciones del panel que precargan el composer (`onRunControl`) no existen
sin chat: el panel sin chat nunca las ofrece.

## Nuevo cliente

Ver la tercera pantalla de la maqueta, derecha. Una ventana (`dialog`) sobre la
lista o la página.

- Campos: nombre y apellido (obligatorio), teléfono, mail, tipo y número de
  documento (DNI o Pasaporte, como hoy). «Cliente desde» se pone en hoy y se
  cambia desde Editar.
- **Aviso de posible repetido** mientras se escribe (con una pausa corta, no
  por tecla): muestra hasta 3 clientes que ya existen y coinciden, con «Ir a
  su ficha». No impide crear. Coincide si:
  - el nombre se parece (similitud de trigramas sobre el nombre plegado, con
    `pg_trgm`, que ya está instalado), o
  - los últimos 8 dígitos del teléfono son iguales a los de su teléfono o de
    uno de sus contactos, o
  - el mail es igual sin mayúsculas, o
  - el número de documento es igual tomando solo los dígitos y letras.
- Botones fijos abajo: «Cancelar», «Crear» (va a la página del cliente nuevo)
  y «Crear y armar un viaje» (va a la página con `?nuevo-viaje=1`).

## Nuevo viaje desde la página del cliente

«Nuevo viaje» abre el formulario de hoy (`NewTripPanel`: recorrido y fechas)
en el panel de la derecha. Al crear:

- cliente sin WhatsApp → el panel muestra el viaje nuevo, para seguir con
  pasajeros y pago;
- cliente con WhatsApp → va al chat con el viaje nuevo elegido, como si se
  hubiera creado desde ahí.

En el servidor, `POST /api/operaciones/viajes` acepta `customerId` como
alternativa a `contactId`: exactamente uno de los dos. Con `customerId` el
viaje se crea para ese cliente sin pasar por `customerFor`. El evento
`trip.created` lleva los `contactIds` del cliente (vacío si no tiene) y el
`customerId`.

## Traer pasajeros

En la pestaña Pasajeros de cualquier viaje (también al lado del chat), si el
cliente tiene otros viajes con pasajeros:

- En el vacío: «Todavía no hay pasajeros», con «Traer de un viaje anterior»
  como botón principal y «Agregar pasajero» al lado.
- Con pasajeros cargados: «Traer de un viaje anterior» al lado del botón
  punteado de «Agregar pasajero».

La ventana lista las personas de los **otros** viajes del cliente, una vez cada
una (ver abajo), en su versión más reciente, con nombre, adulto o menor (con
edad si es menor), el documento con que viajó y su vencimiento («vencido» en
rojo si ya venció), cuántas visas tiene, y de qué viaje sale (`#12 · Japón`).
Vienen marcadas las del viaje anterior más reciente que tenga pasajeros. Las
que ya están en este viaje aparecen deshabilitadas con «Ya está en este
viaje». El botón dice «Traer N pasajeros».

**Qué se copia** de cada una, en una sola transacción: nombre y apellido,
adulto o menor, fecha de nacimiento, país de residencia, todos sus documentos
y todos sus permisos (visas, autorizaciones, residencias), el documento con
que viaja (apuntando a la copia del documento) y si fue elegido a mano.
**Titular**: se copia solo si este viaje no tiene titular todavía; si vienen
dos titulares, queda el primero. **No se copian** las confirmaciones de
requisitos: son de un viaje y un itinerario.

Después de copiar se recalculan los derivados y se reevalúan los requisitos,
igual que después de agregar un pasajero a mano. Las copias son independientes:
editar una no toca el viaje de donde salió.

### Quién es la misma persona

Para «Con quién viaja» y para la ventana de traer pasajeros: dos pasajeros son
la misma persona si tienen el mismo nombre y apellido plegados (minúsculas, sin
tildes, sin espacios repetidos). La versión más reciente es la del viaje creado
más tarde.

## Vincular un chat

### Desde la bandeja

En la ficha de un contacto **que no es de ningún cliente** (`ContactDetails`,
la que se ve cuando el chat no tiene viajes):

- Si los últimos 8 dígitos del teléfono del contacto coinciden con el teléfono
  de un cliente sin ese contacto: una tarjeta «¿Es <Nombre>?» con el nombre,
  cuántos viajes tiene, y «Vincular».
- Siempre: «Vincular a un cliente existente», que abre un buscador de clientes
  (la misma búsqueda de la lista).
- «Nuevo viaje» sigue creando el cliente desde el contacto, como hoy.

Al vincular, el chat muestra los viajes de ese cliente (ya funciona así:
`TripsByContact` sale de `customer_contacts`).

Un contacto que ya es de un cliente no ofrece vincular: eso sería unir dos
clientes, y queda fuera de alcance.

### Desde la página del cliente

«Vincular un chat» abre un buscador de **contactos que todavía no son de ningún
cliente y tienen al menos una conversación individual**, por nombre o teléfono, con su avatar, teléfono y la línea de su
conversación más reciente. Primero aparecen los que coinciden por teléfono. Al
elegir uno se vincula con el `POST /clientes/<id>/contactos` que ya existe.

El buscador lee las tablas públicas del espejo (`contacts`, `conversations`),
como ya hace `operaciones`. No escribe en ellas. No ofrece grupos.

## Cambios en el servidor

Todos en `operaciones`. La forma exacta de las rutas y del JSON queda al
constructor; los nombres siguen el castellano del módulo.

1. **Listar clientes**: búsqueda, chip, orden, cursor, los conteos por chip y,
   por cliente, lo que pide la tabla (contacto a mostrar, cantidad de viajes,
   último destino, viaje abierto más reciente con su estado y salida, avatar).
2. **Crear cliente.** Evento `customer.created`.
3. **Posibles repetidos**: dados nombre, teléfono, mail y documento, hasta 3
   clientes que coinciden.
4. **Detalle del cliente para la página**: los datos que ya da `Customer`, más
   sus contactos con la conversación más reciente de cada uno, sus viajes con
   las columnas de Viajes, y «con quién viaja».
5. **Crear viaje con `customerId`** (ver arriba).
6. **Pasajeros para traer** a un viaje y **copiarlos**. Eventos de pasajero
   como al agregar uno.
7. **`CloseControl`** acepta un control por acción sin mensaje cuando el
   cliente no tiene WhatsApp.
8. **Contactos sin cliente**: buscador por nombre o teléfono, y la sugerencia
   por teléfono para un contacto dado.
9. **Pendientes y la lista de Viajes** ya devuelven `conversationId` vacío
   para un cliente sin chat; necesitan también el `customerId` para abrir el
   panel.

Sin migraciones de esquema obligatorias. Si la búsqueda por teléfono o el
aviso de repetidos lo necesitan, sumar índices queda al constructor.

## Tiempo real

La lista y la página se recargan ante cualquier evento de `operaciones`, como
Viajes. La lista se recarga también al volver a la pestaña del navegador.

## Celular

Como en Viajes: debajo de cierto ancho, la tabla pasa a ser una lista de
renglones (avatar, nombre, contacto abajo, estado del viaje abierto a la
derecha) y la página del cliente apila la columna derecha debajo de los
viajes. El panel del viaje ocupa todo el ancho. Sin scroll horizontal de página.

## Decisiones que tomé yo

Para que las revises:

- «Sin WhatsApp» quiere decir sin un contacto vinculado que tenga chat
  individual, no solo sin contactos vinculados.
- El último movimiento cuenta lo que pasa dentro de los viajes (pasajeros,
  pago, controles), no solo la edición del viaje; no cuenta los mensajes.
- «Marcar hecho» existe solo para clientes sin WhatsApp; con WhatsApp, el
  mensaje sigue siendo la prueba.
- Un viaje de un cliente con WhatsApp se abre siempre en el chat, también
  desde la página del cliente; el panel sobre la página es solo para los que
  no tienen chat.
- En Viajes, el «Sin chat» desactivado pasa a «Abrir viaje» con el mismo
  panel.
- La misma persona se reconoce por nombre y apellido; no por documento,
  porque el documento puede faltar o cambiar entre viajes.
- Vienen marcados los pasajeros del viaje anterior más reciente.
- El aviso de repetido usa similitud de nombre, últimos 8 dígitos del
  teléfono, mail y documento, y no bloquea.
- La sugerencia de vincular por teléfono compara los últimos 8 dígitos: el
  teléfono del cliente se escribe a mano («11 4788-2210», «011 15 4788-2210»)
  y el del contacto viene en E.164.
- Orden por último movimiento.
- Se sacó de la maqueta una columna «Total comprado»: sin un estado de «no se
  concretó», un viaje cerrado puede ser una cotización que no se vendió, y la
  suma mentiría.

## Lo que queda al constructor

- Cómo se suma la entrada de menú para el mismo módulo.
- Cómo se reparte la lectura del viaje entre `TripSidebar` y el panel nuevo.
- La forma de las rutas, del JSON y de los eventos.
- El umbral de similitud del nombre para el aviso de repetidos.
- Los textos de los vacíos que no figuran acá, con las reglas del rediseño.

## Cómo se verifica

Pruebas en Go contra Postgres real (`TEST_DATABASE_URL`):

- crear un cliente sin contacto y crearle un viaje con `customerId`; con
  `contactId` y `customerId` a la vez, o sin ninguno, responde error;
- un control por acción de un cliente sin contactos se cierra sin mensaje; el
  mismo control de un cliente con contacto sigue pidiendo el mensaje;
- cerrar sin mensaje «Requisitos migratorios» guarda `notified`;
- la lista busca por teléfono con y sin formato, por pasajero y sin tildes;
  los chips cuentan bien y combinan con la búsqueda; el orden por último
  movimiento sube a un cliente cuando se edita uno de sus viajes, cuando se
  le carga un pasajero o un pago y cuando se cierra un control;
- un cliente cuyo único contacto vinculado no tiene conversación cuenta como
  sin WhatsApp: su control por acción se cierra sin mensaje;
- traer pasajeros copia documentos, permisos y el documento de viaje
  apuntando a la copia; no copia confirmaciones; respeta un titular existente;
  editar la copia no toca el original; una persona en dos viajes aparece una
  vez, en su versión más reciente;
- el aviso de repetidos encuentra por nombre parecido, teléfono, mail y
  documento;
- la sugerencia de vincular encuentra al cliente por los últimos 8 dígitos y
  no ofrece contactos que ya son de un cliente.

Pruebas de componente:

- la tabla y los chips reflejan la URL y se restauran desde ella;
- tocar un viaje de un cliente con WhatsApp navega al chat; sin WhatsApp abre
  el panel y pone `?viaje=` en la URL;
- «Marcar hecho» aparece en lugar del botón de acción solo sin WhatsApp;
- el aviso de repetido muestra «Ir a su ficha»;
- «Traer de un viaje anterior» marca los del último viaje y deshabilita los
  que ya están.

Al cierre: `go build ./... && go vet ./... && go test ./...` con la base de
test, `npm test`, `npm run typecheck`, y un recorrido en el navegador de la
lista, la página de un cliente con y sin WhatsApp, nuevo cliente, traer
pasajeros y vincular un chat, a 1440 px y a 390 px, sin escribir en la base
real.
