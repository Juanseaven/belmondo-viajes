# Diseño de la pantalla de Viajes

## Objetivo

Darle a la agencia un lugar donde ver todos sus viajes sin tener que estar dentro
de la conversación de un cliente. Hoy la ficha del viaje solo existe al lado del
chat: para saber qué seña vence esta semana hay que abrir una conversación por
una.

La agencia no tiene ningún sistema así, y nadie en ella describió qué buscaría.
Por eso la pantalla no arranca por la búsqueda sino por la pregunta que el
módulo ya sabe responder: **qué hay que hacer y con qué viaje.** Los controles,
las señas y los vencimientos ya están cargados; esta pantalla los junta.

## La regla de lectura

El usuario pidió una cosa por encima de todo: que sea **fácil de leer y útil**.
De ahí salen reglas que valen para toda la pantalla:

1. **Un renglón por viaje, nunca uno por control.** Un viaje con cuatro cosas
   pendientes es un renglón, no cuatro.
2. **Arriba, en grande, qué hay que hacer, dicho con palabras.** «Cobrar la seña
   de USD 800», no «Seña» ni «Atrasado». Abajo, en gris, de quién es: número,
   cliente, destino, salida.
3. **Si un viaje tiene varias cosas pendientes, se muestra la más urgente** y
   «y N cosas más». La lista completa está en la ficha.
4. **Rojo solo para lo atrasado, naranja para lo de esta semana.** Nada en color
   sin una frase que diga por qué (la misma regla de la ficha).
5. **Toda acción lleva al chat**, que es donde se resuelve: la ficha del viaje
   está al lado de la conversación y el envío de mensajes ya existe ahí.

## Dónde vive

Una entrada nueva en el menú lateral: **Viajes**, ruta `/viajes`, ícono ✈. Es la
primera pantalla propia del módulo `operaciones`.

Esto cambia una línea del diseño del módulo, que decía que `operaciones` no tenía
pantalla propia y vivía dentro del espejo. Ahora tiene las dos cosas: su
pantalla, y la ficha que la pantalla del espejo sigue dibujando al lado del chat.
Hay que actualizar el README y la sección «Una asimetría conocida» del diseño del
módulo (`2026-08-20-modulo-operaciones-design.md`).

Dos pestañas: **Pendientes** (la que se abre por defecto) y **Todos los viajes**.
La pestaña elegida y los filtros viven en la URL, como en el espejo, para que una
recarga o un enlace pasado a otra persona muestren lo mismo.

## Pestaña «Pendientes»

Una lista de trabajo que arma el sistema. Nadie la carga ni la ordena a mano. La
pestaña muestra al lado de su nombre cuántos viajes piden algo (los dos primeros
grupos; el tercero no cuenta porque no pide nada).

### Qué entra

Solo viajes que no están cerrados. De cada viaje se miran dos fuentes:

- **Controles abiertos con plazo.** Los derivados no tienen plazo (`due_at` es
  NULL por diseño) y nunca aparecen: se cierran solos al cargar el dato.
- **La seña sin cobrar con vencimiento** (`deposit_due_at` cargado y
  `deposit_paid_at` vacío).

### Los tres grupos

| Grupo | Entra un viaje si… | Color |
|---|---|---|
| **Atrasado** | tiene algún control o seña con plazo anterior a hoy | rojo |
| **Esta semana** | no está atrasado y tiene algo que vence entre hoy y dentro de 7 días | naranja |
| **Salen en los próximos 14 días** | no está en los anteriores y sale entre hoy y dentro de 14 días | neutro |

Cada viaje aparece en **un solo grupo**, el más urgente que le toque. El tercer
grupo existe para que un viaje que sale el lunes esté a la vista aunque esté al
día; su renglón dice «Todo listo» si no le queda nada abierto con plazo, o lo
próximo que vence si lo hay.

«Hoy» es el día calendario en la hora de Argentina (`America/Argentina/Buenos_Aires`),
no en UTC: a las 22 h de Buenos Aires ya es mañana en UTC y la lista no puede
adelantarse.

### Orden

Dentro de cada grupo: primero los que tienen algo crítico, después por plazo más
cercano (el más atrasado primero en «Atrasado»). En el tercer grupo, por fecha de
salida.

### El renglón

- **Línea principal: la tarea más urgente del viaje.** Se elige igual que «Lo que
  sigue» en la ficha: crítico primero, después el plazo más cercano. Una seña
  vencida cuenta como crítica.
- **Cómo se dice la tarea:** con una frase propia de cada renglón de la
  plantilla (ver «La tarea de cada control», abajo). Para la seña: «Cobrar la
  seña de USD 800», o «Cobrar la seña» si no tiene monto cargado.
- **Cuándo**, pegado a la tarea o en la segunda línea: «venció el 18 oct», «vence
  el jueves» (día de la semana si es dentro de los próximos 7 días; fecha si no).
- **Segunda línea, en gris:** `#10482 · Agus Alonso · Cancún · sale 10 dic`. Sin
  destino o sin salida, se omite ese dato, no se escribe «Sin completar».
- **Tercera línea, solo si hace falta:** «y 2 cosas más».
- **Botón «Abrir chat»** (ver más abajo).

- **«y N cosas más»** cuenta las demás cosas del viaje que están atrasadas o
  vencen en los próximos 7 días. Lo que vence más adelante no suma: no pide nada
  esta semana.

No hay paginación: la lista está acotada por la ventana de fechas y por los
viajes abiertos.

### La tarea de cada control

Los títulos de la plantilla describen el dato que se verifica («Se agendó el
check in del vuelo», «Vencimiento de DNI o pasaporte») y no sirven como orden.
Las etiquetas de los botones tampoco: «Avisar al pasajero», leída sola en una
lista, no dice de qué.

Cada renglón de la plantilla que tiene plazo suma un campo `Task` en
`domain/checklist.go`: una frase corta, en imperativo, que se entienda sin
contexto. La plantilla vive en Go y los viajes guardan solo el `item_id`, así
que el campo nuevo no necesita migración y vale también para los viajes ya
abiertos.

| Renglón | Tarea |
|---|---|
| carga-preexistencia | Ofrecer preexistencias |
| carga-comprobante | Pedir el comprobante de pago |
| carga-vouchers | Enviar vouchers y T&C |
| carga-acepto | Pedir que acepte los T&C |
| control-orden | Revisar el orden de nombres y apellidos |
| control-fechas-vuelo | Revisar las fechas de vuelo |
| control-destino | Revisar el destino de la reserva |
| control-equipaje | Revisar el equipaje incluido |
| control-visa | Avisar si necesita visa o vacuna |
| control-hotel-liquidacion | Revisar que el hotel sea el de la liquidación |
| control-hotel-datos | Revisar hotel, fechas y titular de la reserva |
| control-comidas | Revisar el régimen de comidas |
| control-traslados | Informar vuelos y horarios de los traslados |
| control-vto-alojamiento | Registrar el vencimiento del alojamiento |
| control-checkin | Agendar el check in del vuelo de ida |
| control-checkout | Agendar el check in del vuelo de vuelta |
| control-vto-doc | Revisar el vencimiento de los documentos |
| control-flight-plus | Enviar la Flight Plus |
| control-asistencia | Emitir la asistencia médica |
| control-no-reembolsable | Informar el monto no reembolsable |

Un test de la plantilla verifica que todo renglón con plazo tenga tarea.

### Vacío

Sin nada en ningún grupo: «No hay nada pendiente. Los viajes que salen en los
próximos 14 días van a aparecer acá.» Sin ningún viaje en todo el sistema: el
mismo mensaje que explica que los viajes se crean desde la conversación con el
cliente, con un enlace a WhatsApp.

## Pestaña «Todos los viajes»

La consulta: la planilla de viajes, viva.

### Búsqueda y filtros

- **Buscar** en un solo campo: nombre del cliente, nombre o apellido de cualquier
  pasajero, destino, o número de viaje. Sin distinguir mayúsculas ni tildes
  («cancun» encuentra «Cancún»). El índice `pg_trgm` ya está instalado en el
  kernel.
- **Estado**, como chips: Abiertos (todo menos cerrado) · Cotizados · Confirmados
  · En viaje · Cerrados. Por defecto, **Abiertos**.
- **Salida**: próximos 30 días · este mes · el mes que viene · un rango elegido ·
  cualquier fecha (por defecto).
- **Vendedor**: la misma lista de `GET /api/operaciones/vendedores`, más «Sin
  asignar».
- **Con algo atrasado**: el mismo criterio del grupo «Atrasado».

### Columnas

Pocas y en dos líneas, para que entre sin scroll horizontal:

| Columna | Arriba | Abajo, en gris |
|---|---|---|
| # | número | — |
| Cliente | nombre del cliente | «3 pax · Juanse» (pasajeros y vendedor) |
| Destino | destino | estado del viaje |
| Salida | fecha de salida | «al 5 nov» (regreso) |
| Total | USD | estado de la seña: «seña cobrada», «seña vence 10 nov» (naranja si es esta semana), «seña vencida» (rojo) |
| Lo que sigue | la tarea más urgente, con las palabras de Pendientes | — |

Un dato que falta se muestra como «—», no como «Sin completar»: en una tabla de
cuarenta filas la frase repetida se vuelve ruido.

### Orden

Por defecto, **salida más próxima primero**, y los viajes sin fecha de salida al
final. Se puede ordenar también por número (más reciente primero) y por total.
Clic en el encabezado de la columna.

### Paginación

Por keyset, como el espejo (`nextCursor`), de a 50 con carga al llegar al final.
Arriba de la tabla, el total de viajes que cumplen los filtros: «23 viajes».

### Vacío

Con filtros que no dan nada: «Ningún viaje coincide» y un botón «Limpiar
filtros».

### Celular

Debajo de cierto ancho, la tabla pasa a ser una lista de renglones con el mismo
formato que Pendientes (tarea arriba, datos abajo). Sin scroll horizontal de
página en ningún ancho.

## Abrir el chat

Clic en un renglón de Pendientes, en «Abrir chat», o en una fila de Todos los
viajes, lleva a `/mirror?c=<conversationId>` con **ese viaje elegido** en la
ficha (no el que la ficha elegiría por defecto). Hace falta pasar el viaje en la
URL, por ejemplo `&viaje=<tripId>`, y que la ficha lo respete.

**Qué conversación.** Un cliente puede tener varios contactos, y cada contacto
una conversación por línea. Se abre **la conversación más reciente**
(`last_message_at`) de cualquiera de los contactos del cliente. `operaciones`
puede leer `conversations` porque es tabla pública del espejo.

**Sin conversación** (un cliente cuyo contacto ya no tiene conversación con
ninguna línea): el renglón se ve igual pero el botón dice «Sin chat» y está
desactivado, con un título que explica por qué.

## Tiempo real

Las dos pestañas se recargan ante cualquier evento del módulo `operaciones`, como
la ficha. Además, Pendientes se recarga al volver a la pestaña del navegador
(`visibilitychange`): la lista depende del día, y una pestaña abierta desde ayer
mostraría lo de ayer.

## Backend

Dos lecturas nuevas en `operaciones`. Los nombres de ruta siguen el castellano
del módulo:

- `GET /api/operaciones/pendientes` — devuelve los tres grupos ya armados, cada
  viaje con su tarea principal, la cantidad de cosas restantes y su
  `conversationId`. La clasificación se hace en el backend: es la misma regla
  que va a necesitar cualquier aviso futuro y no debe duplicarse en el
  navegador.
- `GET /api/operaciones/viajes` sin `contacto` — el listado con búsqueda,
  filtros, orden y cursor. Con `contacto` sigue respondiendo lo de hoy, que usa
  la ficha.

La forma exacta de los parámetros y del JSON queda para el plan. Lo que no queda
para el plan: la regla de «hoy» en hora de Argentina, la tarea principal elegida
igual que en la ficha, y la conversación más reciente.

## Decisiones tomadas sin consultar

Las tomó quien escribió el diseño; se pueden cambiar sin rehacer el resto:

- los tres grupos y sus ventanas (7 y 14 días);
- la seña vencida cuenta como crítica;
- la redacción de las veinte tareas;
- «Abrir chat» va a la conversación más reciente del cliente;
- en Todos los viajes, por defecto se ven los abiertos ordenados por salida;
- un dato faltante en la tabla es «—»;
- la pestaña Pendientes no pagina.

## Fuera de esta entrega

- La pantalla de **Clientes** (es el paso siguiente del proyecto).
- El tablero por estado y los reportes de ventas.
- Exportar a Excel o imprimir.
- Avisos por mail o notificaciones.
- Asignar tareas a personas: no hay «mis pendientes». Toda la agencia ve todo,
  como en el resto del módulo.

## Verificación

Pruebas en Go contra Postgres real (`TEST_DATABASE_URL`):

- un control vencido ayer pone el viaje en Atrasado; uno que vence en 3 días, en
  Esta semana; uno que vence en 20 días no lo hace aparecer;
- una seña sin cobrar vencida pone el viaje en Atrasado; cobrada, lo saca;
- un viaje sin pendientes que sale en 10 días aparece en el tercer grupo con
  «Todo listo»; uno que sale en 20, no;
- un viaje cerrado nunca aparece;
- un viaje aparece en un solo grupo aunque tenga cosas de dos;
- «hoy» se calcula en hora de Argentina (un control que vence hoy, consultado a
  las 23 h de Buenos Aires, no está atrasado);
- la búsqueda encuentra por pasajero y sin tildes; los filtros combinan;
- la conversación elegida es la más reciente entre todos los contactos del
  cliente.

Pruebas de componente:

- el renglón dice la tarea con palabras y la fecha relativa, no un código ni una
  fecha ISO;
- «y N cosas más» aparece solo cuando corresponde;
- los filtros se reflejan en la URL y se restauran desde ella;
- «Abrir chat» navega con la conversación y el viaje;
- la ficha respeta el viaje que llega por la URL.

Al cierre: `go build ./... && go vet ./... && go test ./...` con la base de
test, `npm test`, `npm run typecheck`, y un recorrido en el navegador de las dos
pestañas, en escritorio y a 390 px, sin escribir en la base real.
