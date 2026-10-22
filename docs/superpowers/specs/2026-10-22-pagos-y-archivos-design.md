# Diseño de pagos registrados y archivos del cliente

Dos agregados al panel del viaje (al lado del chat y sobre Clientes y Viajes) y
a la página del cliente. Son independientes: se pueden construir en cualquier
orden. Maquetas aprobadas (abrir en el navegador):

- [Pagos](maquetas/2026-10-22-pagos-y-archivos/pagos.html)
- [Archivos](maquetas/2026-10-22-pagos-y-archivos/archivos.html)

Las maquetas mandan en lo visual; valen las reglas del rediseño
(`2026-10-21-rediseno-ui-design.md`).

---

## Parte 1 — Pagos registrados

### Por qué

La pestaña Pagos muestra una barra de «cobrado contra total», pero lo único que
la mueve es el tilde de «seña cobrada». Un viaje se paga de a partes, y hoy no
hay dónde anotar cada una: la barra nunca llega al total.

Además, el total en pesos se escribe a mano aparte del total en dólares y del
cambio, y nada controla que coincidan (en un viaje de prueba: USD 5.000 al
cambio de 1.513 decía $ 100.000).

### Palabras

En la pantalla se dice **pago**, no «cobro»: «Pagos», «Registrar pago»,
«Pagado USD 2.800 · Falta USD 2.200», «Pago completo». Lo que hoy es «Editar
pago» pasa a ser **«Editar cotización»**, porque edita el total, la seña y las
condiciones, no un pago.

### Qué es un pago registrado

Una plata que entró, con:

- **monto** (mayor que cero);
- **moneda**: USD o pesos;
- **cambio**, obligatorio si es en pesos; viene puesto el cambio de la
  cotización del viaje y se puede cambiar;
- **equivalente en USD**: el monto si es en dólares; monto ÷ cambio,
  redondeado a centavos, si es en pesos. Se guarda;
- **fecha** (por defecto, hoy en hora de Argentina);
- **forma de pago**, de la misma lista que hoy (Transferencia, Efectivo,
  Tarjeta de crédito, Tarjeta de débito, Mercado Pago); viene puesta la de la
  cotización;
- **nota** opcional, texto libre («comprobante 4421»);
- quién lo cargó y cuándo.

Va en una tabla nueva del módulo `operaciones`, una fila por pago, con clave
foránea al viaje. La tabla `payments` sigue siendo la cotización del viaje (una
fila por viaje); su comentario ya anticipaba esto: «Registrar cada cobro por
separado será otra tabla el día que haga falta».

### La pestaña Pagos

Ver la maqueta, primera pantalla.

- **Tarjeta del total**: total en USD grande, pesos y cambio debajo, la barra
  y «Pagado USD x · Falta USD y». Pagado es la suma de los equivalentes en USD.
  Con lo pagado igual o mayor al total, la barra queda llena en verde y
  «Falta» pasa a decir **«Pago completo»**. Si se pagó de más, «Pagado de más
  USD x» en ocre.
- **Tarjeta «Pagos»**, con «Registrar pago» en el título. Un renglón por pago,
  del más nuevo al más viejo: tilde verde, forma de pago (con «· seña» si es el
  pago con el que se completó la seña), fecha con día de la semana y «cargó
  <nombre>»; a la derecha el monto en USD, y debajo el monto en pesos si se
  pagó en pesos. La nota, si hay, como una línea gris más. Sin pagos: «Todavía
  no hay pagos» y el botón.
- **Tarjeta de la seña**, como hoy, pero sin tilde manual (ver abajo).
- **Forma de pago y no reembolsable**, como hoy.
- Botón **«Editar cotización»**.
- Sin cotización (sin total), como hoy: «Todavía no hay cotización» con
  «Cargar cotización». Se puede registrar un pago igual: la tarjeta de pagos se
  muestra debajo del vacío.

### Registrar un pago

Ver la maqueta, segunda pantalla. Reemplaza el contenido del panel, con el
esquema de Editar (`PanelEditor`): «←» arriba, un solo botón «Registrar pago»
fijo abajo.

- Campos en este orden: moneda (dos botones, USD y Pesos; por defecto USD),
  monto, cambio (solo con pesos), fecha, forma de pago, nota.
- Debajo del monto, una nota gris: «Equivale a USD x. Con este pago faltan USD
  y.» (o «Con este pago se completa el total»).
- Los montos se leen con `parseAmount`, como el formulario de hoy.

### Borrar un pago

Cada renglón tiene un menú o botón «Borrar». Al borrar, el renglón desaparece y
aparece «Borraste el pago de USD x. Deshacer», como al quitar un pasajero.
Deshacer lo vuelve a registrar con los mismos datos. No hay edición: un pago mal
cargado se borra y se carga de nuevo.

### La seña

Sigue siendo un monto y una fecha límite en la cotización, y sigue generando
«Cobrar la seña» en Pendientes con las mismas reglas. Lo que cambia es cómo se
da por cobrada:

- **Se da por cobrada sola** cuando la suma de pagos, en orden de fecha, llega
  al monto de la seña. La fecha de cobro es la del pago que la alcanzó.
- **Tolerancia de USD 1.** Un pago en pesos convertido con un cambio con
  decimales puede quedar unos centavos corto (USD 1.500 al cambio de 1.513,37,
  pagados con $ 2.270.000, dan USD 1.499,96). Para la seña y para «Pago
  completo», una diferencia menor a USD 1 cuenta como alcanzada. «Falta» no
  muestra esos centavos.
- La columna `payments.deposit_paid_at` se mantiene y la calcula el servidor
  al registrar o borrar un pago y al guardar la cotización. Así Pendientes, la
  lista de Viajes y los controles derivados siguen leyendo lo mismo, sin
  cambios.
- El tilde «Seña cobrada» desaparece del formulario de cotización y la API deja
  de aceptarlo.
- Una seña sin monto no se puede dar por cobrada por suma: el formulario pide
  el monto si hay fecha de seña.

**Datos que ya existen.** Una migración convierte cada seña marcada como
cobrada **con monto** en un pago registrado: monto de la seña, USD, fecha de
`deposit_paid_at`, forma de pago de la cotización, nota «Seña». Una seña
marcada como cobrada **sin monto** no se puede convertir (no se sabe cuánto se
pagó): la migración la marca como **cobrada antes de los pagos registrados**
(una columna booleana en `payments`). Para esas, el recálculo conserva
`deposit_paid_at` aunque después se le cargue un monto y no haya pagos, y el
formulario no pide monto. Así no vuelve a aparecer «Cobrar la seña» por una
seña que ya se cobró. La marca se borra sola si alguien carga la seña de nuevo
desde cero (borra el monto y la fecha).

### Total en pesos automático

El total en pesos es **total USD × cambio**, redondeado a pesos enteros. Lo
calcula el servidor al guardar la cotización; si falta alguno de los dos, queda
vacío. El formulario lo muestra como texto («Total en pesos: $ 7.565.000, se
calcula solo»), no como campo. La API deja de aceptar `totalArs`.

Los viajes que ya existen se recalculan en la migración donde hay total y
cambio. El control derivado del total en pesos sigue mirando la misma columna.

### En Viajes y en la página del cliente

La columna Total, debajo del monto, dice hoy el estado de la seña. Pasa a
decir, en este orden de prioridad: «pago completo» (verde) si lo pagado llega
al total; si no, el estado de la seña como hoy; si no hay seña pendiente y hay
pagos, «pagado USD x» en gris. Hace falta sumar lo pagado a cada fila de la
lista (`TripRow`).

### Servidor

- Tabla nueva de pagos registrados y migración de datos (señas y total en
  pesos).
- Registrar un pago y borrarlo. Cada uno recalcula `deposit_paid_at` y los
  derivados del viaje, en la misma transacción, y emite `payment.changed`.
- El detalle del viaje devuelve la lista de pagos (con el nombre de quien los
  cargó) y lo pagado en USD.
- La cotización (`PUT /viajes/{id}/pago`) deja de aceptar `depositPaid` y
  `totalArs`, calcula el total en pesos y recalcula `deposit_paid_at`.
- La lista de Viajes, Pendientes y la página del cliente suman lo pagado en
  USD a cada viaje.

La forma de las rutas y del JSON queda al constructor.

---

## Parte 2 — Archivos del cliente

### Por qué

Las fotos de pasaportes, los comprobantes y los vouchers ya están en los chats,
pero para encontrarlos hay que subir por la conversación. Se quieren ver juntos
y marcar los importantes.

### De dónde salen

De los mensajes del espejo que tienen archivo (`media_assets`) y son de tipo
**imagen, video o documento**. Los audios y stickers no entran. Se juntan los
de **todas las conversaciones individuales de los contactos del cliente** (no
solo del chat abierto); las de grupo no entran, porque ahí manda fotos gente que
no es el cliente. Los mensajes borrados (`deleted_at`) no entran.

`operaciones` lee `messages`, `media_assets`, `conversations` e `instances` del
espejo, como ya hace con otras tablas públicas, y no las escribe. La URL de
cada archivo se pide al endpoint que ya existe, `GET /api/mirror/media/{id}`,
que firma una URL temporal.

### Favoritos

Cualquier archivo se marca con estrella. La estrella es **de la agencia, no de
cada persona**: lo que marca uno lo ven todos. Se guarda en una tabla propia de
`operaciones` (mensaje, quién la marcó, cuándo), con clave foránea al mensaje.
Marcar y desmarcar emiten un evento de `operaciones`.

### La pestaña Archivos del panel del viaje

Ver la maqueta, primera pantalla. Una pestaña más, entre Pagos y Controles, con
la cantidad de archivos. Muestra los archivos del cliente del
viaje (no solo de ese viaje: un archivo no sabe de qué viaje es).

- **Chips**: Todos · Favoritos · Documentos, con cuántos hay en cada uno.
- **Dos listas que paginan por separado**: fotos y videos por un lado,
  documentos por otro, cada una con su cursor. Si fueran una sola, un cliente
  con cientos de fotos nunca llegaría a sus documentos. En «Todos» y
  «Favoritos», la grilla carga de a 60 al llegar al final, y los documentos
  muestran los primeros 5 con «Ver más documentos». En «Documentos» solo está
  la lista, que carga de a 60.
- **Fotos y videos** en grilla de tres columnas, del más nuevo al más viejo,
  agrupados con un título por período («Esta semana», «Este mes», y después el
  nombre del mes, con año si no es el actual). Cada miniatura: la imagen (el
  video, con el ícono de play y su duración), la fecha abajo, la estrella arriba
  a la derecha (se toca sin abrir la foto), y la etiqueta «Agencia» arriba a la
  izquierda si la mandó la agencia; lo del cliente no lleva etiqueta.
- **Documentos** en lista debajo de la grilla, con título «Documentos»: ícono
  con la extensión, nombre del archivo, fecha, tamaño y «del cliente» o «de la
  agencia», y la estrella a la derecha.
- Un archivo que todavía se está bajando de WhatsApp se ve en gris con
  «Cargando…»; uno que falló, en gris con «No se pudo bajar».
- Vacío: «Todavía no hay archivos en los chats de <cliente>». Con «Favoritos»
  y ninguno marcado: «Marcá con la estrella los archivos importantes para
  tenerlos a mano».
- Las miniaturas piden su URL cuando entran en pantalla, no todas de golpe.

### El visor

Ver la maqueta, segunda pantalla. Al tocar una foto o un video se abre en
grande sobre la pantalla:

- arriba, «Foto del cliente» o «Foto de la agencia», fecha y hora, y la línea;
  el texto que acompañó al archivo, si hubo;
- flechas para pasar al anterior y al siguiente dentro de la vista actual
  (respetando el chip elegido);
- abajo, «Favorito» (marca o desmarca), **«Ver en el chat»** (lleva a
  `/mirror?c=<conversación>&m=<mensaje>`, que ya abre el chat en ese mensaje) y
  «Descargar»;
- se cierra con la ✕, con Escape o tocando afuera.

Un documento no abre el visor: se abre en una pestaña nueva del navegador.

### La sección Archivos en la página del cliente

Ver la maqueta, tercera pantalla. Una tarjeta «Archivos» con la cantidad, en la
columna principal debajo de Viajes. Mismo contenido y comportamiento que la
pestaña, con la grilla más ancha (seis columnas). Si hay favoritos, abre en el
chip «Favoritos»; si no, en «Todos». Muestra las primeras 12 fotos o videos y
los primeros 5 documentos; «Ver todos» despliega las dos listas en el lugar.

### Tiempo real

La pestaña y la sección se recargan ante los eventos de `operaciones`
(favoritos) y ante los mensajes nuevos del espejo de los contactos del cliente
(llega una foto nueva, o termina de bajarse una que estaba cargando).

Hoy el espejo avisa cuando llega un mensaje (`message.new`) pero no cuando
termina de bajarse su archivo: ni el chat se actualiza, queda «Descargando
adjunto…» hasta recargar. Se suma al espejo un evento cuando un archivo queda
guardado (con el mensaje y la conversación), y el chat también lo usa para
reemplazar «Descargando adjunto…» por el archivo.

### Servidor

- Listar los archivos de un cliente: chip, cursor, conteos por chip, y por
  archivo: id del archivo, id del mensaje, conversación, tipo, mime, nombre,
  tamaño, ancho y alto, duración, dirección, fecha de WhatsApp, estado de
  descarga, texto del mensaje, línea, y si es favorito.
- Marcar y desmarcar favorito. Solo se aceptan mensajes con archivo de los
  tipos que entran.

La forma de las rutas y del JSON queda al constructor.

---

## Fuera de alcance

- Asignar un archivo a un pasajero o a un pago («este es el pasaporte de Ana»):
  por ahora alcanza con la estrella.
- Subir archivos desde el panel que no vengan del chat.
- Archivos de un chat sin cliente (sin viaje): se siguen viendo en el chat.
- Editar un pago registrado (se borra y se carga de nuevo).
- Pagos en otras monedas que no sean USD o pesos.

## Decisiones que tomé yo

Para que las revises:

- Un pago en pesos se convierte a USD con el cambio de ese pago, que viene
  puesto con el de la cotización.
- La seña se da por cobrada sola por suma de pagos; desaparece el tilde manual.
- Las señas ya marcadas como cobradas con monto pasan a ser un pago; las que no
  tienen monto quedan como estaban.
- No se edita un pago: se borra (con Deshacer) y se vuelve a cargar.
- Se puede registrar un pago sin cotización cargada.
- El total en pesos se redondea a pesos enteros.
- Los archivos son del cliente, no del viaje: la pestaña de cualquier viaje del
  cliente muestra los mismos.
- La estrella es compartida por toda la agencia.
- Audios y stickers no entran en Archivos; grupos tampoco.
- En la página del cliente, la sección abre en Favoritos si hay alguno.
- La seña y el «Pago completo» toleran una diferencia menor a USD 1.
- Las señas viejas cobradas sin monto quedan marcadas y conservan su fecha.
- Fotos y documentos paginan por separado.
- El espejo suma un evento cuando termina de bajarse un archivo, que también
  usa el chat.

## Lo que queda al constructor

- La forma de las rutas, del JSON y de los eventos.
- Cómo se reparte el componente de archivos entre la pestaña y la página.
- Los textos de los vacíos y estados que no figuran acá, con las reglas del
  rediseño.
- Cómo se descarga (enlace con la URL firmada o el endpoint).

## Cómo se verifica

Pruebas en Go contra Postgres real (`TEST_DATABASE_URL`):

- un pago en pesos guarda el equivalente en USD con su cambio; sin cambio, se
  rechaza; un monto cero o negativo, también;
- la seña se da por cobrada con el pago que alcanza su monto, con la fecha de
  ese pago; borrar ese pago la vuelve a dejar sin cobrar; cambiar el monto de
  la seña la recalcula;
- la migración convierte una seña cobrada con monto en un pago y deja intacta
  una sin monto; recalcula el total en pesos;
- la cotización calcula el total en pesos y rechaza `depositPaid` y `totalArs`;
- la lista de Viajes trae lo pagado;
- los archivos de un cliente juntan sus dos contactos, dejan afuera grupos,
  audios, stickers y borrados, y respetan el chip y el cursor;
- marcar favorito lo hace aparecer en «Favoritos» para todos; un mensaje de
  texto no se puede marcar.

Pruebas de componente:

- registrar un pago en pesos muestra el equivalente y lo que falta;
- la barra llega a «Pago completo»;
- borrar un pago ofrece «Deshacer»;
- la grilla agrupa por período y la estrella se marca sin abrir la foto;
- el visor pasa al siguiente y «Ver en el chat» navega al mensaje.

Al cierre: `go build ./... && go vet ./... && go test ./...` con la base de
test, `npm test`, `npm run typecheck`, `next build`, y un recorrido en el
navegador del panel y de la página del cliente, sin escribir en la base real.
