# Diseño del rediseño de la interfaz

Aplica a toda la web (`web/`). No cambia qué hace la app: cambia cómo se ve y
cómo se lee. Donde hace falta un dato que el servidor todavía no entrega, se
dice en «Cambios en el servidor».

Maquetas aprobadas (abrir en el navegador, ancho 1440 px):

- [Bandeja con el viaje abierto y pestaña Requisitos](maquetas/2026-10-21-rediseno-ui/bandeja.html)
- [Pantalla Viajes: Pendientes y Todos los viajes](maquetas/2026-10-21-rediseno-ui/viajes.html)
- [Panel del viaje: Pasajeros, Pagos, Controles y Editar](maquetas/2026-10-21-rediseno-ui/panel-del-viaje.html)

Las maquetas mandan en lo visual. Este documento dice por qué, fija las reglas
que valen para las pantallas que no se maquetaron y marca qué datos son nuevos.

## Por qué

Para llevar la app a producción, tiene que poder usarla cualquier persona de la
agencia sin explicación. Hoy no falla nada, pero hay mucho texto desordenado,
cuesta entenderla y le faltan detalles. La vara es WhatsApp Web: quien la va a
usar ya la conoce, y todo lo que se parezca no hay que aprenderlo.

Lo que más pesa hoy:

- El Resumen del viaje es un formulario abierto: desplegables, campos vacíos
  que dicen «Sin completar» y dos botones de guardar. Se lee como algo para
  llenar, no como el estado de un viaje.
- Cada burbuja enviada desde el teléfono lleva «DESDE EL TELÉFONO» en
  mayúsculas. Es la etiqueta más visible del chat y es la que menos importa.
- Requisitos muestra siempre el párrafo largo de cada renglón. Para saber qué
  falta hay que leerlo todo.
- Los estados (del viaje, de un requisito, de un plazo) se dicen con palabras
  en el mismo gris que el resto.

## Reglas que valen en toda la app

1. **Leer antes que editar.** Los datos se muestran como datos: valor en
   negrita, rótulo chico arriba, ícono a la izquierda. Los formularios aparecen
   al tocar «Editar», nunca abiertos de entrada.
2. **El estado se ve por color antes de leerse.** Etiquetas de color, números
   en las pestañas, íconos de cumple, falta o verificar, puntos en «Lo que
   sigue». El color siempre acompaña una palabra que dice lo mismo, nunca la
   reemplaza.
3. **Lo que ya existe en WhatsApp se ve como en WhatsApp.** Lista de chats,
   burbujas, tildes, separador de fecha y de no leídos.
4. **Un vacío dice qué falta y ofrece la acción.** «Todavía no hay cotización»
   con «Cargar cotización», no ocho campos vacíos. En las tablas, lo que falta
   dice qué falta («Sin cotizar», «Sin fecha») en gris, no una raya.
5. **Los plazos en palabras** («Hace 1 día», «Mañana», «Viernes», «En 6 días»)
   con la fecha corta debajo o al lado. Las fechas sueltas llevan día de la
   semana (`jue 29/10/2026`).
6. **El texto largo se pliega.** Lo que explica (el detalle de un requisito,
   las fuentes) queda a un toque, no a la vista.
7. **Un formulario, un botón de guardar**, fijo abajo, con un aviso de qué
   cambió al lado.

### Lo que se mantiene de hoy

La dirección visual de `globals.css` sigue: la tipografía del sistema (Segoe
UI, la de WhatsApp Web), dos pesos (400 y 600), el neutro frío de fondo, el
verde como único acento de acción y selección, el ocre para «requiere acción»,
el rojo para «vencido» y las cifras tabulares. Los valores de color de las
maquetas son los de `globals.css`.

### Colores de estado del viaje

Los mismos en el panel, en la tabla de Viajes y en cualquier otro lugar donde
aparezca el estado:

| Estado     | Fondo / texto                     |
|------------|-----------------------------------|
| Consulta   | gris (`--canvas` / `--ink-2`)     |
| Cotizado   | azul (`#eef1f6` / `#3c5a85`)      |
| Confirmado | verde (`--green-tint` / `--green-strong`) |
| En viaje   | violeta (`#f2eef8` / `#6a4c93`)   |
| Cerrado    | sin fondo, texto `--ink-4`        |

Azul y violeta son nuevos: van como tokens en `globals.css`
(`--blue`, `--blue-tint`, `--blue-line`, `--violet`, …).

### Avatares

Cuando no hay foto, iniciales sobre un color sacado del nombre (un hash
estable a una paleta de 7 tonos apagados, los de la maqueta). El mismo contacto
tiene siempre el mismo color en la bandeja, en Viajes y en Pasajeros.

## Bandeja

Ver maqueta `bandeja.html`.

**Lista de chats**

- Encabezado: título «Chats», el filtro de línea como píldora («Todas las
  líneas ▾») y «+» para conectar línea. Se va el número suelto que no decía de
  qué era y el menú «…».
- Chips: Todos, No leídos con su número.
- Fila: avatar de 48 px, nombre, hora (en verde y negrita si hay no leídos),
  vista previa en una línea con quién escribió en un grupo («Natalie: Dale»),
  tildes si el último lo mandó la agencia, ícono y duración si es audio,
  número de no leídos en verde. Se va el renglón «Sin leer».
- Si el contacto tiene un viaje abierto, un tercer renglón con una etiqueta
  «✈ #4 Estados Unidos» y, si su próximo control está atrasado, otra en rojo
  «Atrasado». El dato ya existe: `useTrips().byContact` trae el viaje con
  `nextControl`, no la cantidad de controles atrasados; por eso la etiqueta no
  lleva número (en la maqueta dice «1 pendiente»).
- Números de teléfono con formato (`+54 9 11 2612-6437`) donde el nombre es el
  número.

**Chat**

- Cabecera: avatar, nombre, teléfono con formato y la línea por la que entra
  («● Línea Juanse»). A la derecha: buscar y mostrar u ocultar el panel del
  viaje.
- Barra del viaje: ícono de avión, «Viaje #4 · Estados Unidos», fechas con
  ícono de calendario, pasajeros con ícono de persona. Si hay controles
  críticos atrasados, a la derecha una etiqueta roja «N control(es)
  atrasado(s)» (el título del control está en el panel).
- Burbujas: las de la agencia en verde claro, colita en la primera de cada
  tanda del mismo lado, hora dentro de la burbuja abajo a la derecha.
  - Enviado desde el teléfono: ícono de celular chico al lado de la hora, con
    `title="Enviado desde el teléfono"`. Se va la etiqueta en mayúsculas.
  - Enviado desde el panel: el nombre de quien lo escribió arriba, en verde,
    solo en la primera de la tanda.
  - Tildes según `message.status`: uno gris enviado, dos grises entregado, dos
    azules leído. Los estados «Enviando» y error siguen como hoy.
  - En grupos, el nombre de quien escribe arriba, solo en la primera de la
    tanda.
- Separador de fecha en minúscula con el día («Lunes 19 de octubre»; «Hoy» y
  «Ayer» cuando corresponde).
- Separador «N mensajes sin leer» antes del primer no leído, usando
  `conversation.unread` al abrir el chat.
- Composer: campo redondeado, clip a la izquierda, enviar redondo verde a la
  derecha. Se va la ayuda «Enter para enviar · Shift+Enter para salto».

**Contacto sin viaje**

La ficha del contacto (`ContactDetails`) sigue las mismas reglas: datos a la
vista con íconos y, como vacío, «Este contacto no tiene viajes» con «Nuevo
viaje».

## Panel del viaje

Ver maquetas `bandeja.html` y `panel-del-viaje.html`.

**Encabezado (todas las pestañas)**

- «Viaje #4» y al lado la etiqueta del estado con su color y una flecha: al
  tocarla abre un menú con los cinco estados. Se guarda al elegir, como hoy.
- Debajo: destino y «vende <nombre>». El vendedor se cambia desde Editar.
- Si el contacto tiene más de un viaje, el número del viaje abre el menú para
  cambiar de viaje.
- A la derecha: «+» nuevo viaje y «✕» ocultar.

**Pestañas**: Resumen, Pasajeros (cantidad), Requisitos (cantidad en
«falta», en rojo; sin número si no falta nada; «verificar» no suma), Pagos,
Controles (cantidad abierta).

**Resumen**

- Bloque de datos en una grilla con bordes: recorrido («Argentina → Estados
  Unidos», ancho completo), salida, regreso, cliente, pasajeros
  («1 · adulto»).
- «Editar viaje y cliente» abre la pantalla de edición.
- Tarjetas en este orden: «Lo que sigue» (el próximo control con su botón y
  «N controles más» plegados), «Requisitos» (barra de avance en tres colores y
  leyenda; «Ver detalle» va a la pestaña), «Pago» (total y seña en dos
  renglones; «Cargar» o «Editar» va a la pestaña), «Cliente» (teléfono, mail,
  documento, cliente desde).

**Requisitos**

- Una tarjeta por pasajero y país, con bandera, nombre del país, pasajero y
  documento, y «N de M listos» (listos = «cumple»; las recomendaciones no
  cuentan). Lo que no es de un país (la autorización de menor, las
  recomendaciones) va en una tarjeta «General» del pasajero, antes de los
  países; lo que es del viaje entero (falta el itinerario), en una tarjeta
  arriba de todo.
- Un renglón por requisito: ícono redondo (verde tilde, rojo cruz, ocre «!»),
  nombre, etiqueta de estado, y «Pide» y «Tiene» en una línea cada uno.
- El renglón se despliega con la flecha: muestra el texto completo de hoy
  (todas las partes de `detail`) y las fuentes al pie.
- La acción al lado del problema: «Confirmar» donde hoy está, y si falta un
  permiso, «Cargar visa» que lleva a Pasajeros con ese pasajero abierto.
- Lo que hoy es una nota fija («Lo que está en verificar…») va como nota chica
  al pie con un ícono de información.
- La vista previa sin pasajeros sigue la misma forma, sin estado.

**Pasajeros**

- Una tarjeta por pasajero: avatar, «Apellido, Nombre», etiquetas (Titular,
  Adulto/a, «Menor · 9 años»), y con qué documento viaja y cuándo vence. Si
  vence antes del regreso, en ocre debajo.
- Al abrirla (flecha) se despliegan Documentos (el que se usa resaltado en
  verde, elegible tocándolo, como el radio de hoy) y Visas y permisos, cada uno
  con «+ Agregar».
- «Editar datos» abre el formulario del pasajero. «Quitar pasajero» queda
  separado, en rojo, y conserva el «Deshacer» de hoy.
- «Agregar pasajero» al final, como botón punteado de ancho completo.

**Pagos**

- Con cotización: tarjeta con el total en grande, los pesos y el cambio
  debajo, una barra de cobrado contra total y «Cobrado USD x · Falta USD y».
  Cobrado es la seña si está marcada como cobrada; si no, cero.
- Tarjeta de seña: con tilde verde y fecha si se cobró; ocre con «vence el …»
  si no; roja si venció.
- Forma de pago y no reembolsable como lista de dos columnas.
- «Editar pago» abre el formulario de hoy con el mismo esquema de Editar
  (botón fijo abajo, volver arriba).
- Sin cotización: «Todavía no hay cotización» y «Cargar cotización».

**Controles**

- Dos grupos: «Por hacer · N» y «Hechos · N».
- Por hacer: círculo con el color del plazo (rojo atrasado, ocre esta semana,
  gris más adelante), título, «Importante» en rojo si es crítico, plazo en
  palabras, y el botón de la acción («Confirmar», o el `actionLabel`).
- Hechos: tilde gris, título en gris regular, quién y cuándo, «Reabrir» donde
  hoy existe.

**Editar viaje y cliente**

- Reemplaza el contenido del panel (no es un modal), con «←» para volver.
- Secciones: Recorrido (sale de; destinos y escalas como lista con «✕»;
  «Agregar destino» y «Agregar escala»), Fechas (salida y regreso; «Fechas en
  cada país» plegado), Vendedor, Cliente (nombre, teléfono, mail, tipo y número
  de documento).
- Un único «Guardar cambios» fijo abajo que guarda lo que cambió del viaje, el
  itinerario y el cliente, con «Cancelar» y un aviso de qué cambió. Si algo
  falla, dice qué no se guardó y deja lo demás guardado.
- Volver con cambios sin guardar pregunta antes de descartarlos.

**Nuevo viaje** (`NewTripPanel`) usa el mismo esquema que Editar.

## Pantalla Viajes

Ver maqueta `viajes.html`.

**Pestañas**: «Pendientes» con el número en rojo si hay algo atrasado y en
gris si no; «Todos los viajes».

**Pendientes**

- Grupos con un cuadrado del color (rojo atrasado, ocre esta semana, gris los
  que salen) y cuántos viajes tiene cada uno.
- Renglón: avatar del cliente; la tarea en negrita; debajo, el nombre del
  cliente en negrita y «#4 · Estados Unidos · sale el jue 29 oct»; «y N cosas
  más» como etiqueta chica; a la derecha, el plazo en palabras con la fecha
  debajo, y «Abrir chat».

**Todos los viajes**

- Filtros en dos renglones. Arriba: buscar, y los desplegables que dicen lo
  elegido («Salida: Próximos 30 días ▾», «Vendedor: Todos ▾»). Abajo: los
  estados como chips con cuántos viajes hay en cada uno, un separador y «Con
  algo atrasado» en rojo.
- Debajo: «8 viajes · ordenados por salida».
- Tabla: # en gris; Cliente con avatar y «N pasajeros · vendedor» abajo;
  Destino con la etiqueta de estado abajo; Salida con día de la semana y
  «vuelve el …»; Total alineado a la derecha con el estado de la seña en su
  color; Lo que sigue con un punto de color y el plazo en palabras; flecha a
  la derecha. La fila se resalta al pasar el mouse.

## Pantallas sin maqueta

Login, conectar línea (`LineDialog`), filtro de líneas (`LineFilter`),
resultados de búsqueda (`SearchResults`) y la barra lateral (`Shell`) se
ajustan con las reglas de arriba, sin cambiar lo que hacen. En la barra
lateral, el punto verde sobre el ícono de la bandeja cuando hay no leídos.

## Cambios en el servidor

Dos, chicos, porque las maquetas muestran datos que hoy no llegan:

1. **Último mensaje de cada conversación** (módulo mirror): sumar a la
   conversación la dirección del último mensaje, su estado (para los tildes) y,
   en grupos, el nombre de quien lo escribió. Hoy solo hay
   `last_message_preview`.
2. **Cantidad de viajes por estado** (módulo operaciones): la lista de viajes
   devuelve, además del total, cuántos hay en cada estado con los demás
   filtros aplicados, para los números de los chips.

## Fuera de alcance

- **Textos cortos de «Pide» y «Tiene»** (por ejemplo «Visa de turista
  B1/B2»): hoy el sistema solo guarda el texto largo. Mientras tanto, el
  renglón muestra la primera línea de cada parte cortada a dos renglones, y el
  texto completo al desplegar. Pedirle al buscador la versión corta es un
  cambio aparte.
- **Filtro «Con viaje»** en la bandeja: la lista viene paginada y el filtro
  necesitaría al servidor. Queda para después.
- **Quién marcó la seña como cobrada**: el pago no lo guarda; se muestra solo
  la fecha.
- Modo oscuro y uso en celular.

## Decisiones que tomé yo

Para que las revises:

- Los colores de estado del viaje (tabla de arriba).
- La alerta de la barra del viaje dice cuántos controles están atrasados, no
  el título: el título completo no entra.
- En la lista de chats, la etiqueta de atraso dice «Atrasado» sin número (ver
  Bandeja).
- El número de Requisitos cuenta solo lo que falta, no lo que hay que
  verificar.
- El separador «sin leer» se calcula con el número de no leídos al abrir el
  chat, contando hacia atrás los mensajes recibidos.
- Las banderas se dibujan con una librería de SVG (`country-flag-icons`),
  porque en Windows los emojis de bandera se ven como dos letras.
- El formato de teléfono se hace a mano para números argentinos; los demás se
  muestran con «+» y el número sin cambios. No se suma `libphonenumber`.
- El vendedor pasa del encabezado a Editar: se cambia poco y ocupaba un
  desplegable a la vista.
- Se va «Enter para enviar · Shift+Enter para salto» del composer: WhatsApp
  funciona igual y no lo dice.

## Lo que queda al constructor

- Cómo partir los componentes y el CSS (hoy `globals.css` tiene 1300 líneas;
  conviene que cada pantalla nueva no lo haga crecer en la misma proporción).
- Los textos de los estados vacíos que no figuran acá, con las reglas de
  arriba.
- El orden de trabajo. Lo que más se ve primero: bandeja y panel del viaje,
  después Viajes, después el resto.

## Cómo se verifica

- Los tests existentes siguen pasando; los que buscan textos que cambian
  («Desde el teléfono», «Sin leer», «Guardar viaje») se actualizan.
- `npm run typecheck`, `npm test` y `go test ./...` en verde.
- Cada pantalla se compara con su maqueta en la app corriendo, a 1440 px y a
  1280 px.
