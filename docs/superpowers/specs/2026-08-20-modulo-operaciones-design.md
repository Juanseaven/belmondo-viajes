# Diseño del módulo operaciones

## Objetivo

Llevar a la aplicación real el expediente del viaje que hoy solo existe como
maqueta en `/mirror?demo=1`, con persistencia, endpoints y tiempo real.

El trabajo reemplaza dos planillas en papel de la agencia —la de carga de
reserva y la de control de reserva— y le da a la empresa su propia base de
clientes y viajes, visible desde la conversación de WhatsApp donde el vendedor
ya trabaja todo el día.

Nace un módulo nuevo, `operaciones`. El módulo `mirror` no se toca: ni una columna,
ni una migración.

## La regla de módulos, revisada

Hasta hoy el README y `modules/module.go` decían que **los módulos no se conocen
entre sí**. La intención era que agregar un módulo fuera fácil; la redacción
prohibió de más. La ceguera no compra facilidad: compra duplicación. Un módulo
de operaciones ciego necesitaría su propia copia de cada contacto, y su evidencia
sería un uuid suelto que nada garantiza.

La regla pasa a ser:

> **El kernel no conoce a ningún módulo. Los módulos sí se conocen, siempre en
> una sola dirección.**
>
> 1. Un módulo puede **leer** tablas de otro. **Escribirlas, nunca**: las
>    invariantes que el dueño mantiene en código no se saltean desde afuera.
> 2. El módulo dueño **declara qué tablas son públicas**, en un comentario de su
>    migración. Tocar una columna pública es cambiar un contrato: se busca quién
>    la lee antes.
> 3. Las claves foráneas entre módulos están permitidas hacia esas tablas
>    públicas. La integridad referencial la da la base o no la da nadie.
> 4. El grafo de dependencias es acíclico y las dependencias se pasan a mano al
>    construir, en `cmd/api/main.go`.

Lo que hacía fácil agregar un módulo se conserva entero: una línea en `main.go`,
migraciones numeradas sin coordinar con nadie, rutas montadas solas.

El costo asumido: una tabla leída es un contrato que el compilador no verifica.
Si algún día un renombre duele, la salida es una vista en el módulo dueño que
absorba el cambio. No hace falta todavía.

`mirror` declara públicas `contacts`, `conversations` y `messages`.

## Principio operativo

Una planilla digital de treinta y un casilleros es teatro: alguien tilda los
treinta y uno sin mirar y el error que la hoja quería evitar ocurre igual, ahora
con un registro que afirma lo contrario.

**El estado de un control es una consecuencia, no una declaración.** De ahí
salen tres formas de cerrar, y la forma la fija la plantilla, nunca el usuario:

- **`derivado`** — el control se cumple porque **el dato existe**. El sistema lo
  cierra solo y nadie toca nada. «Destino» no es un control: es un campo. Tildar
  «Destino» en un viaje que tiene destino es la definición del teatro.
- **`accion`** — el sistema ejecuta la acción y el control queda cerrado con la
  evidencia enlazada: el mensaje que avisó de la visa, que envió los vouchers,
  que informó el monto no reembolsable.
- **`manual`** — se confirma a mano, registrando quién y cuándo. Queda para lo
  único que lo merece: **comparar nuestro registro contra el del operador**. El
  régimen de comidas, el orden de los nombres, el hotel de la liquidación. Eso
  el sistema no lo puede saber; una persona sí.

De 31 renglones, **11 se cierran solos, 8 se cierran mandando un mensaje y 12 se
tocan a mano**. Una lista de 12 se lee. Una de 31 con la mitad obvia se tilda en
bloque.

## El modelo

Siete tablas, todas del módulo `operaciones`.

### Cliente

El cliente es de la agencia, no de WhatsApp. Un contacto de WhatsApp es quien
escribió; un cliente es a quien se le vende. No son lo mismo: el mismo cliente
escribe desde su número y desde el de la mujer, «Familia Rossi» es un grupo y no
un cliente, y quien compra por teléfono no tiene contacto alguno.

`customers` guarda nombre, mail, teléfono, documento y desde cuándo es cliente
—todo lo que WhatsApp no da—, con autor y fecha de alta.

`customer_contacts` es el puente hacia `mirror.contacts`. `contact_id` es
**único**: un contacto pertenece a un solo cliente, y un cliente puede tener
varios contactos. El puente vive acá y no como columna en `mirror.contacts`
porque escribir en tablas ajenas está prohibido; la consecuencia feliz es que
desinstalar `operaciones` deja el espejo exactamente como estaba.

El vínculo no cuesta un paso: al abrir el primer viaje desde una conversación,
si ese contacto no tiene cliente se crea uno con el nombre del contacto y quedan
vinculados ahí mismo.

### Viaje

En pantalla y en el código se dice **viaje** —`trips`—, no «operación» ni
«expediente». La palabra que usa la agencia es la que va en la interfaz, y el
código habla el mismo idioma para que nadie traduzca entre lo que lee y lo que
programa.

Guarda número visible, estado (`cotizado`, `confirmado`, `en viaje`, `cerrado`),
destino, fecha de salida, fecha de regreso, el cliente, el vendedor —una
persona, no la línea de WhatsApp— y quién lo creó y cuándo.

**No hay columna de «viaje activo».** Un cliente puede tener legítimamente dos a
la vez: uno confirmado y otro cotizándose. El banner muestra el más reciente sin
cerrar y deja cambiar. Una columna `is_active` sería un dato más que puede
mentir.

### Pasajeros

`passengers` absorbe de las planillas los nombres completos, los apellidos, su
orden, el titular de la reserva y el vencimiento del documento.

Cada pasajero registra su nombre **tal como figura en el documento**, si es el
titular, si es adulto o menor, y el tipo, número y vencimiento de su documento.

El resumen «2 ad + 1 men» deja de ser un conteo y pasa a ser una lista de
personas. Y como el vencimiento es un dato y no un casillero, el control del
documento vencido se dispara solo.

### Pagos

`payments`, una fila por viaje: total en USD, cambio del día, valor en pesos,
método, **seña con su vencimiento y si está paga**, y monto no reembolsable.

Es la mitad del expediente que más rápido le cambia el día a la agencia: «quién
me debe la seña y para cuándo» es la pregunta que se hacen todas las mañanas.

Registrar cada cobro por separado —cuotas, pagos parciales— será una tabla
`payment_entries` el día que haga falta. Hoy no hace falta.

### Controles y evidencia

`controls` son los renglones que cada viaje copia de la plantilla al nacer:
título, etapa (`carga` o `control`), criticidad, estado, forma de cierre y
`due_at` calculado.

`control_evidence` guarda quién cerró, cuándo, y el `message_id` hacia
`mirror.messages` cuando el cierre fue por acción. Va en tabla aparte para que
reabrir un control no borre la historia de que alguna vez se cerró y con qué.

## La plantilla

La plantilla **no es una tabla**. Vive en Go como constante y cada viaje se
lleva su copia al nacer. Cambiar el proceso es un deploy; los viajes ya abiertos
conservan intactos los renglones con los que nacieron, así que el versionado se
resuelve solo. No hay pantalla de administración de plantillas, y eso es una
decisión de usabilidad tanto como de arquitectura: nada que administrar es nada
que aprender.

Cada renglón lleva su plazo. Los de `carga` cuentan **desde el alta del viaje**;
los de `control`, **hacia atrás desde la fecha de salida**. Si se corre la fecha
de salida, todos los vencimientos se corren solos.

### Hoja de carga

| Renglón | Cierre | Se cierra con / Plazo | Crítico |
|---|---|---|---|
| Nombres completos | derivado | un pasajero con nombre | |
| Apellidos | derivado | un pasajero con apellido | |
| Destino | derivado | `trips.destination` | |
| Fecha de salida | derivado | `trips.depart_at` | |
| Fecha de llegada | derivado | `trips.return_at` | |
| Tel y mail del pax cargados | derivado | `customers.email` y `phone` | |
| Valor en USD | derivado | `payments.total_usd` | |
| Valor en pesos | derivado | `payments.total_ars` | |
| Cambio del día | derivado | `payments.rate` | |
| Método de pago | derivado | `payments.method` | |
| Se ofreció preexistencia | acción | alta + 2 días | |
| Comprobante de pago y de carga en el portal | acción | alta + 3 días | |
| Vouchers y T&C enviados al pax | acción | salida − 7 días | |
| El pax aceptó los T&C | acción | alta + 5 días | sí |

### Hoja de control

| Renglón | Cierre | Plazo | Crítico |
|---|---|---|---|
| Nombre y apellido del titular | derivado | un pasajero titular | |
| Orden de los nombres y apellidos | manual | salida − 30 días | sí |
| Fechas de vuelo | manual | salida − 30 días | sí |
| Destino | manual | salida − 30 días | sí |
| Equipaje incluido | manual | salida − 30 días | |
| Aviso al pax si requiere visa o vacuna | acción | salida − 45 días | sí |
| El hotel reservado es el de la liquidación | manual | salida − 30 días | |
| Hotel, fechas de reserva y titular | manual | salida − 30 días | |
| Régimen de comidas | manual | salida − 30 días | |
| Traslados: vuelos y horarios informados | manual | salida − 15 días | |
| Vencimiento del alojamiento | manual | salida − 30 días | |
| Se agendó el check in del vuelo | manual | salida − 2 días | |
| Se agendó el check out del vuelo | manual | regreso − 2 días | |
| Vencimiento de DNI o pasaporte | acción | salida − 60 días | sí |
| Flight Plus emitida y enviada | acción | salida − 15 días | |
| Asistencia médica emitida | manual | salida − 15 días | |
| Monto no reembolsable informado | acción | salida − 30 días | sí |

El control del documento tiene un comportamiento propio que sale gratis de tener
el dato: si algún pasajero tiene un documento que vence **antes de la fecha de
regreso**, el control se marca crítico solo, sin que nadie lo mire.

## Qué se ve en pantalla

Todo pasa donde ya trabajan. No hay pantalla nueva, ni menú nuevo, ni lugar al
que haya que ir: es la bandeja de siempre con tres zonas que ganan información.

### Fila de la bandeja

Cada fila muestra a qué viaje pertenece la conversación, y un chip derivado de
los controles: el control crítico abierto más próximo a vencer; si no hay
crítico, el abierto más próximo; y si no hay ninguno abierto, el estado del
viaje.

El chip **dice qué está atrasado**, con las mismas palabras del control. No dice
«atrasado» a secas: eso obliga a abrir la conversación para saber por qué, que
es exactamente el trabajo que la columna venía a evitar.

### Banner del hilo

Sobre los mensajes, una franja con el viaje activo —número, destino, fechas,
pasajeros y total— y el control crítico si lo hay. Conecta las dos mitades de la
pantalla: mientras se conversa, el viaje está a la vista.

### Panel derecho

Reemplaza el panel de detalles actual por: **Resumen**, **Pasajeros**, **Pagos**
y **Controles**. Los datos del contacto y del cliente —teléfono, mail, cliente
desde, atendido por— quedan como un bloque dentro de Resumen.

El panel **no muestra veinte controles: muestra el próximo paso.** Arriba, lo
que hay que hacer ahora, con su botón. El resto, plegado tras «Ver los N
controles restantes». La lista completa existe para auditar, no para recorrerla
todos los días.

**No hay barra de progreso ni porcentaje.** Una barra al 64 % le pide a alguien
que la lleve al 100 %, y el camino más corto para eso es tildar. Además el
promedio esconde lo que importa: un viaje al 90 % con el pasaporte vencido está
peor que uno al 60 % con todo lo crítico resuelto.

### Las reglas de que nadie quede trabado

Son requisito, no aspiración, y se verifican en las pruebas de componente:

1. **Cada renglón abierto dice qué hacer, no qué verificar.** No «Flight Plus
   emitida y enviada al pasajero» sino el botón **«Enviar Flight Plus»**. Los
   manuales dicen contra qué comparar: no «Régimen de comidas» sino **«¿El
   régimen que reservaste es el que cotizaste?»**, con *Coincide* / *No
   coincide*.
2. **El vacío enseña.** Un viaje recién creado no muestra listas vacías: muestra
   «Cargá los pasajeros para empezar» con el botón al lado.
3. **Se carga de a pedazos.** Cada bloque se edita en su lugar y se guarda solo.
   El alta son tres campos. Un pasajero se agrega con nombre y apellido; el
   documento se completa cuando llegue la foto. **Ningún formulario bloquea.**
4. **Deshacer en vez de preguntar.** Nada de «¿estás seguro?»: se hace y queda
   «Listo — deshacer». La única excepción es mandarle un mensaje a un cliente,
   que en WhatsApp no se deshace.
5. **Nada rojo sin explicación.**
6. **Cero configuración.**

### El cierre por acción

El botón **no envía**: precarga el mensaje en el cuadro de texto, con el nombre
del titular ya reemplazado. El vendedor lo ajusta al tono de la conversación y
envía. Recién cuando el mensaje sale de verdad, el control se cierra enlazado a
él.

El texto sale del sistema pero pasa por una persona. Un clic accidental que le
manda un texto genérico a alguien que está pagando un viaje no se puede
deshacer.

Como consecuencia, **el backend no necesita enviar nada**: el envío usa el
endpoint de `mirror` que ya existe, y el frontend cierra el control con el id
del mensaje que ese endpoint devuelve.

### Toda la agencia toca todo

Cualquiera atiende cualquier conversación y toca cualquier viaje. De ahí:

- **Todo dice quién y cuándo**, visible en la interfaz y no escondido en un log.
  Nadie se acuerda de un viaje que no vendió.
- **Tiempo real, y no como lujo.** Si dos personas miran el mismo viaje y una
  cierra «avisar por visa», la otra tiene que verlo desaparecer sin refrescar, o
  le manda al cliente el mismo mensaje dos veces. `operaciones` publica sus eventos
  por el hub de websockets del kernel.
- **No hay «mis viajes».** El vendedor queda registrado como dato; no filtra ni
  esconde nada.

## Los endpoints

Todo bajo `/api/operaciones/`, con sesión.

```
GET    /viajes?contacto=            los viajes de un contacto
POST   /viajes                      alta mínima: contacto, destino, salida, regreso
GET    /viajes/{id}                 el expediente completo
PATCH  /viajes/{id}                 destino, fechas, estado, vendedor
POST   /viajes/{id}/pasajeros
PATCH  /pasajeros/{id}   ·   DELETE /pasajeros/{id}
PUT    /viajes/{id}/pago            total, cambio, método, seña y su vencimiento
POST   /controles/{id}/cerrar       { mensajeId? }
POST   /controles/{id}/reabrir
GET    /clientes/{id}    ·   PATCH  /clientes/{id}
POST   /clientes/{id}/contactos     vincular otro número al mismo cliente
GET    /bandeja?contactos=id1,id2   por contacto: viaje activo y control que sigue
```

`POST /viajes` hace tres cosas en una transacción: crea el cliente si el
contacto no tenía, crea el viaje, y materializa los 31 controles con su
vencimiento calculado.

`PATCH /viajes/{id}` recalcula los vencimientos de la etapa `control` cuando
cambia la fecha de salida, y reevalúa todos los controles derivados.

`GET /bandeja` existe porque la lista de conversaciones sigue siendo de
`mirror`, con su paginación, su búsqueda y sus no-leídos. `operaciones` **podría**
servir la bandeja entera con un JOIN —la regla nueva se lo permite— pero
duplicaría esa lógica. Responde solo «para estos contactos, qué viaje y qué
control sigue»: una llamada más sobre los ~30 contactos de la página visible,
cero lógica duplicada.

## Una asimetría conocida

En el backend, `operaciones` depende de `mirror`. En el frontend es al revés: la
pantalla del espejo importa los componentes de `operaciones` para dibujar el banner,
el panel y el chip.

No es un ciclo —son capas distintas— pero es una asimetría y conviene tenerla
escrita. Si algún día molesta, la salida es que la pantalla del espejo deje de
ser «de mirror» y pase a ser una pantalla de la aplicación que compone los dos
módulos. No hace falta ahora.

**Actualización (2026-10-20).** Este diseño decía que `operaciones` no aparecía
en el menú. Ya no es así: tiene su pantalla, **Viajes**
(`2026-10-20-pantalla-viajes-design.md`), además de la ficha que la pantalla del
espejo sigue dibujando al lado del chat. La asimetría de arriba se mantiene.

## Alcance

Entra en este trabajo:

- el módulo `operaciones` con sus siete tablas, sus migraciones y sus endpoints;
- clientes con su puente hacia los contactos de WhatsApp;
- viajes con alta mínima de tres campos;
- pasajeros;
- pagos con seña y vencimiento;
- los 31 controles con sus tres formas de cierre y sus vencimientos calculados;
- el banner, el panel de cuatro pestañas y el chip de la fila, en `/mirror`;
- los eventos de websocket del módulo;
- la regla de módulos revisada, en el README y en `modules/module.go`;
- **el borrado de `/mirror?demo=1`** y de sus datos ficticios, que existían
  justamente como referencia hasta que esto tuviera backend real.

Queda afuera:

- servicios y documentos del viaje (segunda entrega);
- registrar cobros individuales (`payment_entries`);
- la página de listado de viajes y la de clientes;
- tareas, calendario, reportes y plantillas editables;
- cualquier integración con el sistema del operador.

## Riesgos

**La carga de datos es el riesgo real, no el diseño.** Todo esto vive de que
alguien tipee pasajeros, documentos y fechas. La lapicera es más rápida que
cualquier formulario: si cargar cuesta más que el papel, vuelven al papel y
queda una aplicación vacía llena de controles rojos.

No podemos ganarle al papel en velocidad de entrada. Le ganamos en lo que el
papel no puede: avisar de un vencimiento, tener la prueba del aviso, no perder
el comprobante en el scroll. Por eso el alta es de tres campos, nada más allá de
esos tres es obligatorio, y ningún formulario bloquea.

**Lo que esto no resuelve**, para que no sorprenda: no evita retipear los
nombres en el portal del operador, que es donde se va el tiempo de verdad. Eso
solo lo arregla una integración con el proveedor.

## Verificación

Pruebas en Go sobre migraciones reales, con `TEST_DATABASE_URL`, como ya hace el
proyecto:

- el alta materializa los 31 controles con sus vencimientos;
- un control derivado se cierra al cargar el dato que lo cumple, y se reabre si
  el dato se borra;
- un cierre por acción exige un `mensajeId` que exista y pertenezca a la
  conversación del viaje;
- mover la fecha de salida corre los vencimientos de la etapa `control`;
- un documento que vence antes del regreso marca el control como crítico;
- el alta sobre un contacto sin cliente crea el cliente y lo vincula.

Pruebas de componente:

- «Lo que sigue» muestra lo crítico primero y pliega el resto;
- un viaje sin pasajeros muestra qué cargar, no una lista vacía;
- el chip dice qué está atrasado, no solo que lo está;
- el botón de un control por acción precarga el composer y **no** envía.

Al cierre: `go build ./... && go vet ./... && go test ./...`, `npm test`,
`npm run typecheck`, `npm run build`, e inspección visual de `/mirror` sin
scroll horizontal de página.

Los tests del frontend se corren con `npm test`, que fija el entorno jsdom.
Invocar `vitest` directamente sin ese entorno falla con `document is not
defined`.
