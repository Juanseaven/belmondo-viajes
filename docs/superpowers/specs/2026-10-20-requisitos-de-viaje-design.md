# Diseño de los requisitos de viaje

## Objetivo

Que la ficha del viaje diga sola, por cada pasajero, qué necesita para entrar a
cada país del itinerario, qué tiene, qué está mal y qué falta, y que el aviso al
cliente salga armado con eso.

Hoy la agencia lo hace a mano: consulta TravelDoc o Timatic, lee, y se olvida.
El control «Aviso al pax si requiere visa o vacuna» se cierra mandando un
mensaje genérico sin que el sistema sepa si hacía falta visa. El de
«Vencimiento de DNI o pasaporte» compara contra el regreso, no contra lo que
pide el destino.

La idea del proyecto es que esto sea **automático**. Nadie carga reglas. Las
personas cargan datos de los pasajeros (por ahora a mano; más adelante un agente
desde el chat), y el sistema pone las reglas y hace el cruce.

## Lo que no se negocia

1. **Las reglas son 100% automáticas.** No hay formulario de reglas ni nadie que
   las apruebe. Salen de las fuentes y entran en vigencia solas.
2. **Lo que el sistema no sabe lo dice.** Un campo que ninguna fuente cubre, o
   en el que las fuentes no coinciden, queda en **«verificar»**, nunca en «OK».
   Un control abierto de más le cuesta a la agencia un minuto; un «OK» falso le
   cuesta un pasajero varado en el aeropuerto.
3. **La persona resuelve el viaje, no la regla.** El «verificar» aparece en la
   ficha del viaje, en el pasajero al que afecta, y ahí se resuelve. No existe
   una cola de reglas esperando a alguien.
4. **El motor solo lee campos.** No sabe si un dato lo cargó una persona o el
   agente que vendrá después. Sumar el agente no cambia el motor.

## Las tres partes

1. **Datos** (módulo `operaciones`): itinerario del viaje; fecha de nacimiento,
   residencia, documentos y visas de cada pasajero.
2. **Reglas** (módulo nuevo `requisitos`): qué pide cada país a cada
   nacionalidad, sacado de las fuentes automáticamente.
3. **Evaluación** (módulo `operaciones`): el cruce por pasajero y país, los
   controles que alimenta y el aviso al cliente.

`operaciones` depende de `requisitos`, nunca al revés (ver «Arquitectura»).

---

## Parte 1: datos

### El itinerario

`trips.destination` sigue existiendo como nombre para mostrar («Cancún»,
«Europa clásica»). Se agrega el itinerario: una tabla `trip_countries` con una
fila por país.

| Campo | Qué es |
|---|---|
| `trip_id` | el viaje |
| `position` | orden en el itinerario |
| `country` | código ISO 3166-1 alfa-2 |
| `role` | `destino` o `escala` |
| `arrive_at`, `leave_at` | opcionales; si faltan, se usan las fechas del viaje |

Es una lista y no un país porque un circuito por Europa pasa por varios y cada
uno se evalúa por separado. De las escalas alcanza el país; el aeropuerto queda
para más adelante.

**Se asume que todos los viajes salen de Argentina.** El origen no se modela; la
regla de salida de menores (ver más abajo) se aplica a todos.

### El pasajero

Se agregan a `passengers`:

- **`birth_date`**: de ahí sale si es menor a la fecha de salida (menos de 18
  años). Cuando está cargada, `kind` deja de elegirse a mano y se deriva; sin
  fecha, `kind` sigue funcionando como hoy.
- **`residence_country`**: ISO alfa-2, por defecto `AR`. Pesa en algunas
  exenciones.
- **`travel_document_id`**: con qué documento viaja **en este viaje**. Si el
  pasajero tiene un solo documento, es ese.

### Documentos

Tabla `passenger_documents`, varios por pasajero:

| Campo | Qué es |
|---|---|
| `passenger_id` | el pasajero |
| `type` | `DNI` o `Pasaporte` |
| `country` | país emisor, ISO alfa-2 |
| `number` | número |
| `issued_at` | fecha de emisión, opcional (la pide Schengen) |
| `expires_at` | vencimiento |

**La nacionalidad sale del documento**, no es un campo aparte: un pasaporte
italiano dice que es italiano. El documento con el que viaja define la
nacionalidad con la que se evalúa. Eso resuelve la doble nacionalidad, que en
una agencia argentina es común (pasaportes italianos y españoles).

### Visas y autorizaciones que ya tiene

Tabla `passenger_permits`:

| Campo | Qué es |
|---|---|
| `passenger_id` | el pasajero |
| `kind` | `visa`, `autorizacion_electronica` o `residencia` |
| `country` | país que la otorgó, ISO alfa-2 |
| `label` | opcional, texto libre («B1/B2», «ESTA») |
| `valid_from`, `expires_at` | vigencia |

Con esto se resuelven solas cosas como «Canadá pide visa, salvo que tengas visa
de EE.UU. vigente».

### Migración

Los documentos que hoy están en `passengers.document_type`, `document_number` y
`document_expires_at` pasan a `passenger_documents` como documentos argentinos
(`country = 'AR'`) y quedan como `travel_document_id` de su pasajero. Las
columnas viejas se eliminan en la misma entrega, junto con todo el código que
las lee (incluido `sqlRefreshDerived`).

Los viajes existentes no tienen itinerario. Hasta que alguien lo cargue, su
evaluación es «falta cargar el país de destino» (ver «Evaluación»). No se
intenta adivinar el país desde el texto de `destination`.

### Lo que no cambia todavía

Los pasajeros siguen siendo por viaje: quien repite viaje se vuelve a cargar. El
módulo de clientes, que guardará a cada persona una sola vez, viene después.
Por eso documentos y visas cuelgan del pasajero en tablas propias y no como
columnas: mudarlos a la persona será mover dos tablas, no rediseñar.

La carga es a mano desde la ficha. El agente que leerá el chat y las fotos del
pasaporte llenará estos mismos campos más adelante.

---

## Parte 2: reglas (módulo `requisitos`)

### La regla

Una regla por **nacionalidad + país**, las dos en ISO alfa-2. Tiene una lista
cerrada de campos. Cada campo tiene:

- un **valor**, o **`verificar`** con el motivo («ninguna fuente lo cubre», «las
  fuentes no coinciden»);
- sus **fuentes**: cuál, la cita textual de donde salió, la URL y la revisión.

| Campo | Valores |
|---|---|
| `documento` | `dni_o_pasaporte`, `pasaporte` |
| `visa` | `no_requiere`, `autorizacion_electronica`, `visa_electronica`, `visa_a_la_llegada`, `visa_consular`, `no_admitido` |
| `estadia_max_dias` | número |
| `vigencia` | cantidad de días o meses + desde cuándo se cuenta (`llegada`, `salida_del_pais`) |
| `emision_max_anios` | antigüedad máxima del pasaporte (Schengen: 10) |
| `exenciones` | lista: «si tiene un permiso vigente (`visa`, `autorizacion_electronica` o `residencia`) de `country` → `visa` pasa a X». El `residence_country` del pasajero cuenta como un permiso de `residencia` de ese país |
| `vacunas` | lista de vacunas exigidas y a quién (todos / quienes vienen de país de riesgo) |
| `transito` | para cuando el país es escala: `sin_requisito`, `requiere_visa`, `verificar` |

No hay campo de seguro: la agencia emite asistencia médica en todos los viajes y
ya tiene su control (`control-asistencia`).

Estados de la regla: **`buscando`** (todavía no hay datos), **`vigente`**, y
**`sin_fuentes`** (las fuentes fallaron y nunca hubo datos; todos los campos
cuentan como `verificar`).

### Fuentes

**1. Wikipedia, la principal.** Las páginas «Visa requirements for X citizens»
(una por nacionalidad), leídas por la API de MediaWiki. Licencia CC BY-SA: la
ficha muestra «Fuente: Wikipedia» con el enlace. Se toman:

- la fila del país en la tabla de visas (categoría, días, notas; cada fila suele
  citar Timatic y fuentes oficiales);
- la sección de documentos aceptados sin pasaporte («Passport not required»);
- las tablas de vacunación.

Se guarda el texto tomado y el número de revisión de la página. Las peticiones
llevan un User-Agent con un contacto de la agencia, que se configura (ver
«Arquitectura»).

**Espera de 72 horas.** Cualquiera puede editar Wikipedia, y las ediciones malas
se suelen revertir en horas. Por eso el texto de una fila (o sección) se usa
solo si es **idéntico en la revisión actual y en la que estaba vigente hace 72
horas** (la API de MediaWiki da la revisión vigente a una fecha). Si difieren,
la regla sigue con el último texto aceptado y esa fila se vuelve a mirar cada
día hasta que se estabilice. En una regla nueva, si difieren, no se usa ninguna
de las dos versiones: la de hace 72 horas puede ser justo la vandalizada que
después se revirtió. La regla queda sin fuentes (todo en `verificar`) y se
revisa cada día hasta que la fila se estabilice. (Corrección posterior a la
aprobación, a partir de una revisión de seguridad.)

**2. Passport Index, la segunda opinión.** La matriz que publica el repositorio
[imorte/passport-index-data](https://github.com/imorte/passport-index-data)
(licencia MIT), archivo `passport-index-tidy-iso2.csv`: tres columnas
(pasaporte, destino, requisito) en ISO alfa-2, para todas las combinaciones. Los
valores son días sin visa (`90`), `visa free`, `visa on arrival`, `eta`,
`e-visa`, `visa required`, `no admission` y `-1` (mismo país). Ejemplos reales
para pasaporte argentino: Brasil `90`, Reino Unido `eta`, Australia `e-visa`,
EE.UU. `visa required`.

- Se descarga el archivo en cada pasada semanal y se guarda una copia; si un día
  no está, se usa la última copia. **No se accede a passportindex.org.**
- Solo aporta la categoría de visa y los días; no trae vigencia, DNI,
  exenciones ni vacunas. No pasa por Claude: es estructurado.
- Los datos tienen la fecha del último cambio del archivo (hoy, febrero de
  2026). El autor los saca de passportindex.org; la licencia MIT es la del
  repositorio.
- **Va detrás de un interruptor de configuración, encendido por defecto.** Se
  registra cada conflicto que genera (destino, qué dijo cada fuente, fecha de
  los datos) para poder decidir si apagarlo cuando quede viejo.

**iVisa, desactivado.** Su servidor MCP público (`https://www.ivisa.com/mcp`)
devuelve visa, estadía y vigencia estructuradas para destinos donde vende una
visa. Sus términos prohíben el acceso automatizado y reproducir su contenido, y
el MCP está pensado para que un asistente le responda a una persona, no para
alimentar una base de reglas. Queda implementable detrás de un interruptor
**apagado por defecto**, para encenderlo solo si iVisa lo autoriza por escrito.
Esta entrega no necesita implementarlo; alcanza con dejar el lugar de una fuente
más.

**3. Constantes en el código.** Tres reglas que ninguna fuente cubre bien y que
casi no cambian. Viven en Go, como la plantilla de controles, cada una con su
fuente citada en el código. Nadie las carga: viajan con el sistema.

- **Schengen:** pasaporte vigente al menos 3 meses después de la salida prevista
  del espacio Schengen y emitido en los últimos 10 años. Aplica a los 29 países
  del espacio, para toda nacionalidad que no sea de la UE, el EEE o Suiza.
- **DNI argentino:** alcanza con el DNI para los países que lista la sección
  «Passport not required» de Wikipedia (Mercosur salvo Guyana, Comunidad
  Andina, Chile). Es constante porque es estable y porque es justo lo que más
  vende la agencia; si Wikipedia dice lo mismo, mejor.
- **Salida de menores desde Argentina:** todo pasajero menor necesita la
  autorización de viaje, salvo que viaje con ambos padres. El sistema no sabe
  con quién viaja, así que es un requisito que una persona confirma (ver
  «Confirmaciones»).

### Extracción

Claude lee el texto tomado de Wikipedia y completa los campos de la regla con
**la cita textual de cada uno**. Si no encuentra algo, lo
deja en `verificar`; no adivina.

- Modelo `claude-opus-5-5`, SDK oficial de Anthropic para Go, con salida
  estructurada contra el esquema de la regla y el fallback del lado del
  servidor activado (`fallbacks: "default"` con la beta
  `server-side-fallback-2026-07-01`). Si la extracción falla igual, la regla
  sigue como estaba (o queda `sin_fuentes` si es nueva) y se reintenta en la
  próxima pasada.
- **Comprobación de citas en código:** cada cita tiene que aparecer literalmente
  en el texto de la fuente **y decir lo que se extrajo**: «no requiere visa»
  necesita una cita con «not required» o «visa free»; una estadía de 90 días,
  una cita con el 90; una exención, una cita que nombre al país del permiso
  (que además tiene que existir en el catálogo). Si no, ese campo pasa a
  `verificar` (o se descarta, si facilitaba el viaje). Así ni una cita inventada
  ni una cita real que no viene al caso sostienen un valor.
- El prompt aclara que el texto de Wikipedia es un dato y que no hay que seguir
  instrucciones que aparezcan adentro.
- Las constantes del código no pasan por Claude.

### Cuándo coinciden

La comparación entre fuentes **la hace código común**, no Claude:

- **Wikipedia sola alcanza.** Si Passport Index no tiene la combinación, o está
  apagado, no es un conflicto.
- **Conflicto** es cuando las dos fuentes dicen algo y no coincide. Por ejemplo:
  Wikipedia dice `no_requiere` y Passport Index dice `visa required`. El campo
  queda en `verificar` con el motivo «las fuentes no coinciden».
- Para comparar `visa` hace falta una tabla de equivalencias entre las
  categorías de Wikipedia, los valores de Passport Index y los valores de la
  regla (por ejemplo, días o `visa free` → `no_requiere`; `eta` →
  `autorizacion_electronica`). Lo que no esté en la tabla cuenta como conflicto.
- **Exenciones:** Passport Index da la regla general («Canadá: visa required»)
  y Wikipedia puede agregar una exención en las notas («con visa de EE.UU.
  alcanza la eTA»). Eso no es conflicto: la categoría general coincide, y la
  exención es un campo aparte.
- `estadia_max_dias`: si solo una fuente lo dice, vale ese. Si las dos lo dicen
  distinto, gana **el más exigente** (menos días) y se anota el motivo. Ser más
  exigente nunca deja a nadie varado. `vigencia` solo sale de Wikipedia y de las
  constantes.
- Las constantes del código ganan sobre las fuentes en sus campos.

### Cuándo corre

- **Primera vez:** cuando la evaluación de un viaje necesita una combinación
  nacionalidad + país sin regla. La regla nace en `buscando`, se busca en el
  momento (segundos), y la ficha muestra «buscando requisitos» hasta que llega.
- **Cada semana:** para cada regla, se vuelven a bajar las fuentes. **Solo se
  llama a Claude si el texto tomado cambió** (se compara un hash) y pasó la
  espera de 72 horas. El CSV de Passport Index se baja una vez por pasada y se
  compara contra todas las reglas.
- **Si una fuente se cae:** se mantiene la última regla vigente. Si nunca hubo
  regla, queda `sin_fuentes` y se reintenta en la próxima pasada.
- Cada cambio de una regla vigente queda guardado (versión anterior y nueva),
  para poder ver por qué un viaje cambió de estado.

**Costo esperado:** una extracción ronda los USD 0,20 con Opus 5.5. Como solo se
extrae cuando cambia la fuente, son centavos a un par de dólares por mes. Se
guarda el consumo de tokens de cada extracción para medirlo.

---

## Parte 3: evaluación (módulo `operaciones`)

### Qué se evalúa

Por cada pasajero y cada país del itinerario, con la regla de la nacionalidad de
su documento de viaje. Si el país es el mismo que emitió el documento, no se
evalúa nada (es su país).

Cada requisito termina en uno de cuatro estados: **cumple**, **falta**,
**verificar** o **no aplica**.

| Requisito | Cómo se evalúa |
|---|---|
| Documento | viaja con DNI y la regla pide pasaporte → falta |
| Vigencia | vencimiento del documento contra la vigencia que pide la regla; sin vencimiento cargado → falta el dato; regla en `verificar` → verificar |
| Antigüedad | emisión contra `emision_max_anios`; sin fecha de emisión cuando la regla la pide → falta el dato |
| Visa | `autorizacion_electronica`, `visa_electronica` o `visa_consular`: se busca un permiso vigente de ese país durante la estadía → cumple o falta; las exenciones se aplican con los permisos cargados («tiene visa de EE.UU. vigente»). `visa_a_la_llegada`: cumple, con la frase «se tramita al llegar». `no_admitido`: falta, con la frase «este país no admite pasajeros con este documento»; no se confirma, se resuelve viajando con otro documento |
| Estadía | días en el país contra `estadia_max_dias` → si se pasa, falta |
| Vacunas | exigida para todos → falta hasta confirmar; solo para quienes vienen de país de riesgo → verificar (Argentina es de riesgo parcial según la OMS) |
| Tránsito | solo para escalas: si el país no pide visa a esa nacionalidad, cumple; si la pide, verificar (puede hacer falta visa de tránsito) |
| Menor | menor a la salida → falta hasta confirmar la autorización o que viaja con ambos padres |

Un viaje sin itinerario cargado tiene un único requisito: «falta cargar el país
de destino». Un pasajero sin documento de viaje: «falta cargar el documento».

### Dónde se guarda

El resultado se guarda en una tabla de `operaciones`, `trip_requirements`: una
fila por pasajero, país y requisito, con estado, la frase que se muestra y la
versión de la regla usada. Se recalcula en Go:

- después de cada escritura que toque pasajeros, documentos, permisos,
  itinerario o fechas (donde hoy se llama a `RefreshDerived`);
- cuando `requisitos` avisa que cambió una regla: se re-evalúan todos los viajes
  no cerrados que la usan.

Guardarlo, en vez de calcularlo al leer, es lo que permite que
`sqlRefreshDerived` cierre los controles con una consulta, como ya hace.

### Confirmaciones

Hay cosas que el sistema no puede saber: si el menor tiene la autorización, si
el pasajero tiene la vacuna, o qué dice de verdad un campo en `verificar`. Para
eso, `requirement_confirmations`: quién confirmó, cuándo, para qué pasajero,
país y requisito, y opcionalmente el mensaje del chat que lo prueba.

- Se pueden confirmar: **verificar** (cualquiera), **menor** y **vacunas**.
- Confirmar significa «lo revisé y este pasajero cumple», y la línea pasa a
  cumple. No se carga ningún valor. Si al revisar resulta que no cumple (por
  ejemplo, sí necesita visa), no se confirma: la línea sigue en verificar hasta
  que el pasajero la tenga.
- **No** se puede confirmar un «falta» que es un dato ausente: si falta la visa
  o el vencimiento, se carga el dato. Confirmar ahí sería el tilde que el
  principio del módulo prohíbe.
- Una confirmación de un campo en `verificar` queda atada al valor de la regla
  en ese momento. Si la regla cambia ese campo, la confirmación deja de valer y
  el requisito se vuelve a evaluar.

### Los controles

**`control-visa`** pasa a llamarse **«Requisitos migratorios»** (tarea:
«Avisar los requisitos migratorios»). Sigue siendo por acción, crítico, a 45
días de la salida, y gana un dato derivado nuevo, `pasajero.requisitos`:

- se cierra solo cuando todos los requisitos de todos los pasajeros están en
  cumple o no aplica (salvo vigencia y antigüedad, que son del control de abajo);
- el aviso también lo cierra, como hoy, **pero solo si no queda nada en
  verificar, buscando o `sin_fuentes`**. Eso es trabajo de la agencia, y el
  aviso prueba que se le avisó al cliente, no que la agencia verificó. Con algo
  pendiente de verificar, el mensaje se puede mandar igual, pero el control
  sigue abierto;
- un cierre por aviso no lo deshace el mismo estado que se avisó. Si después
  aparece un requisito nuevo en falta o verificar que no estaba en el aviso (por
  un cambio de regla, de itinerario o de pasajeros), el control se reabre: el
  aviso probaba otra cosa. El panel de requisitos sigue mostrando lo que falta
  aunque el control esté cerrado;
- el mensaje deja de ser genérico (ver «El aviso»).

**`control-vto-doc`** («Vencimiento de DNI o pasaporte») pasa a comparar contra
lo que pide el destino: se cierra cuando el documento de viaje de cada pasajero
cumple la vigencia y la antigüedad de todos los países del itinerario. **Donde
la regla no sabe la vigencia, se usa el criterio de hoy** (que venza después del
regreso), para que este control no se vuelva más ruidoso que ahora; el
«verificar» de la vigencia queda en el panel y en «Requisitos migratorios».

Una migración aplica los dos cambios a los viajes ya abiertos, como hizo la
`0003`.

### El aviso

El botón de «Requisitos migratorios» precarga un mensaje armado con lo que
falta, por pasajero y en palabras:

> Juan, te paso lo que hace falta para el viaje a Canadá:
> • María: no tiene visa cargada. Con la visa de EE.UU. vigente alcanza con la
>   eTA, que se tramita online.
> • Tomás: al ser menor, necesita la autorización de viaje si no viaja con
>   ambos padres.
> Cualquier duda, avisame.

Lo que está en `verificar` no va al mensaje: es tarea de la agencia, no del
cliente. **El mensaje dice «está todo en orden» solo si todos los requisitos
están en cumple o no aplica.** Si no falta nada pero queda algo por verificar o
buscando, dice que la agencia le confirma los requisitos en breve. La redacción
final es del builder.

---

## Pantalla

Todo dentro de la ficha que ya existe al lado del chat. Valen las reglas de
lectura de la pantalla de Viajes: palabras y no códigos, y nada en color sin
una frase que diga por qué.

- **Viaje:** editor del itinerario (países, destino o escala, fechas
  opcionales). Los países se eligen por nombre en español, no por código.
- **Pasajero** (`PassengersTab`): fecha de nacimiento, residencia, la lista de
  documentos con «viaja con este», y la lista de visas y autorizaciones.
- **Sección nueva «Requisitos»:** agrupada por pasajero y, dentro, por país. Una
  línea por requisito: estado, la frase («Pasaporte vence el 03/12; Schengen
  pide hasta el 15/03») y la fuente con su enlace. Muestra «buscando
  requisitos…» mientras una regla está en `buscando`. En las líneas
  confirmables, el botón «Confirmar» (con el texto de lo que se confirma); en
  las confirmadas, quién y cuándo.
- Los cambios llegan en tiempo real por el websocket existente: la ficha pasa de
  «buscando» al resultado sin recargar.

---

## Arquitectura

**Módulo `requisitos`**, nuevo:

- Tablas propias: las reglas (pública, la lee `operaciones`), el texto y la
  revisión de cada fuente por regla, el historial de versiones, y el consumo de
  tokens de cada extracción.
- Un servicio Go que `operaciones` recibe al construirse: pedir las reglas de un
  conjunto de combinaciones (las que faltan nacen en `buscando` y se encolan).
- Un proceso de fondo propio, como `mirror/purge.go`: atiende la cola de
  primeras búsquedas, hace la pasada semanal y revisa cada día las filas de
  Wikipedia en espera de 72 horas.
- Las fuentes detrás de una interfaz común (Wikipedia, Passport Index y, apagada,
  iVisa), para que sumar una fuente más, como el agente futuro, no toque la
  combinación.
- Para avisar un cambio de regla sin depender de `operaciones`, expone un
  gancho que `cmd/api/main.go` conecta con la re-evaluación de `operaciones`.
  El grafo de imports sigue siendo acíclico: `operaciones` importa
  `requisitos`, nunca al revés.
- Tiene el catálogo de países: ISO alfa-2, nombre en español, el nombre que usa
  Wikipedia en la tabla y el gentilicio en inglés del título de la página.

**Configuración nueva:** `ANTHROPIC_API_KEY`, el contacto para el User-Agent de
Wikipedia, y los interruptores de Passport Index (encendido por defecto) e iVisa
(apagado por defecto). Sin API key, el módulo arranca igual y todas las reglas nuevas quedan
en `sin_fuentes` (todo en `verificar`): el resto del sistema no se cae por esto.

**Lo que queda para el plan:** la forma exacta de las tablas, los endpoints y el
JSON; cómo se recorta la fila y las secciones del wikitext; el prompt y el
esquema de extracción; la tabla de equivalencias de categorías; el contenido
del catálogo de países.

## Decisiones tomadas sin consultar

Las tomó quien escribió el diseño; se pueden cambiar sin rehacer el resto:

- la nacionalidad sale del documento y no es un campo del pasajero;
- todos los viajes salen de Argentina;
- Wikipedia sola alcanza; el conflicto es solo cuando las dos fuentes dicen algo
  distinto;
- en estadía gana la fuente más exigente;
- la espera de 72 horas para aceptar un cambio de Wikipedia;
- Passport Index como segunda opinión con interruptor; iVisa apagado hasta que
  lo autorice;
- se sacaron las páginas oficiales por destino (pedían una lista de URLs que
  alguien cargara) y el campo de seguro;
- la vacuna «solo para quienes vienen de país de riesgo» va a `verificar`;
- en escalas, una visa exigida se trata como «tránsito: verificar»;
- `control-vto-doc` conserva el criterio de hoy donde la regla no sabe la
  vigencia;
- lo que está en `verificar` no va en el mensaje al cliente;
- el aviso no cierra «Requisitos migratorios» mientras quede algo por verificar,
  y un requisito nuevo después del aviso lo reabre;
- confirmar es «cumple», sin cargar un valor;
- visa a la llegada cuenta como cumple; «no admitido» es falta sin confirmación;
- la residencia cuenta como un permiso de residencia para las exenciones;
- los viajes existentes no adivinan el país desde el texto del destino;
- pasada semanal.

## Riesgos

- **Wikipedia la edita cualquiera.** Mitigan: la espera de 72 horas, la
  comprobación de citas, Passport Index como contraste en la visa, el historial
  de versiones y que el control se cierre con aviso al cliente, no con la regla.
- **Passport Index puede quedar viejo.** Lo mantiene una sola persona y hoy
  tiene ocho meses. Un dato viejo da un conflicto, que es `verificar`: seguro,
  pero ruidoso. Por eso se registran los conflictos y se puede apagar.
- **La tabla de vacunas de Wikipedia es de 2019.** La ficha muestra la fecha de
  la fuente; las vacunas condicionadas ya van a `verificar`.
- **Demora:** un cambio repentino puede tardar hasta una semana en entrar, y
  Wikipedia puede tardar en reflejarlo. La aerolínea decide con Timatic. Por
  eso el aviso al cliente sigue siendo lo que cierra el control y el panel no
  dice «podés viajar», dice qué encontró y de dónde.

## Fuera de esta entrega

- El agente que carga datos de pasajeros desde el chat y las fotos del
  pasaporte.
- Que el agente cree o corrija reglas. El diseño deja el lugar: una regla es
  una fila con fuentes, y el agente podrá ser una fuente más.
- El módulo de clientes (personas guardadas una sola vez).
- Aeropuertos de escala y reglas finas de tránsito.
- Registrar contra Timatic o seats.aero si la regla coincidía.
- Pedidos de acceso a APIs con licencia (Timatic, TravelDoc, Sherpa, seats.aero).

## Verificación

Pruebas en Go del motor de evaluación (puras, sin base):

- DNI argentino a Brasil cumple; a EE.UU. falta pasaporte;
- pasaporte argentino a Canadá sin permisos: falta visa; con visa de EE.UU.
  vigente: cumple por la exención; con visa de EE.UU. vencida: falta;
- pasaporte italiano de un pasajero con doble nacionalidad se evalúa con la
  regla italiana;
- Schengen: pasaporte que vence 2 meses después de la salida del espacio, falta;
  emitido hace 11 años, falta; sin fecha de emisión, falta el dato;
- un campo en `verificar` da `verificar`, nunca cumple;
- menor a la salida (cumple 18 durante el viaje) cuenta como menor;
- una escala en un país que pide visa da «tránsito: verificar»;
- una confirmación de un `verificar` deja de valer cuando la regla cambia ese
  campo;
- un «falta» por dato ausente no se puede confirmar.

Pruebas en Go de la combinación de fuentes:

- Passport Index sin la combinación, o apagado, y Wikipedia «sin visa» →
  `no_requiere`, sin conflicto;
- Wikipedia «sin visa» y Passport Index `visa required` → `verificar`, y el
  conflicto queda registrado;
- Canadá: Passport Index `visa required` y Wikipedia «visa required» con la
  exención de la eTA → sin conflicto, exención cargada;
- estadías distintas → gana la menor, con el motivo anotado;
- una fila de Wikipedia que cambió hace menos de 72 horas no se usa; la regla
  sigue con el texto anterior;
- una cita que no aparece en el texto de la fuente → `verificar`;
- sin API key → la regla queda `sin_fuentes` y todo en `verificar`.

Pruebas contra Postgres real (`TEST_DATABASE_URL`):

- «Requisitos migratorios» se cierra solo cuando todo cumple y se reabre si deja
  de cumplir; el aviso lo cierra solo si no queda nada por verificar ni
  buscando; un cierre por aviso no se deshace por lo ya avisado, pero sí por un
  requisito nuevo en falta o verificar;
- el mensaje nunca dice «todo en orden» con requisitos en verificar, buscando o
  `sin_fuentes`;
- `control-vto-doc` usa la vigencia del destino donde la hay y la del regreso
  donde no;
- cambiar una regla re-evalúa los viajes abiertos que la usan y no los cerrados;
- la migración pasa los documentos viejos a `passenger_documents` como
  argentinos y deja el `travel_document_id` puesto.

Pruebas de componente:

- la sección «Requisitos» dice cada requisito con palabras y muestra la fuente;
- «buscando requisitos…» mientras la regla está en `buscando`;
- el botón «Confirmar» aparece solo en lo confirmable.

Prueba manual con las fuentes reales: crear viajes de pasajeros argentinos a
Brasil, EE.UU., Canadá, España y Japón, y comparar lo que dice la ficha contra
una consulta manual en seats.aero.
