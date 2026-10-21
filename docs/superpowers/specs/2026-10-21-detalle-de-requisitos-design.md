# Diseño del detalle de los requisitos

Agregado al [diseño de los requisitos de viaje](2026-10-20-requisitos-de-viaje-design.md).
Todo lo de ese documento sigue valiendo salvo donde este lo cambia.

## Objetivo

Que cada renglón de la pestaña «Requisitos» se pueda leer como la respuesta que
la agencia le da al cliente, sin abrir otra página: qué pide el país y por qué,
qué tiene el pasajero, qué hay que hacer y, si hace falta un trámite, cómo es.

Hoy cada renglón es una frase suelta:

```
Cumple  El Pasaporte alcanza para entrar a Estados Unidos
Falta   Faltan las fechas del viaje para medir la vigencia del documento
Falta   Estados Unidos pide visa y no está cargada        Fuente: Wikipedia
```

Para una agencia que trabaja en serio, eso no alcanza. Pasó en la primera
prueba: al ver «vigente toda la estadía» para Estados Unidos, la primera
pregunta fue «¿y lo de los 6 meses?». La respuesta (Estados Unidos pide 6 meses,
pero la Argentina está en la lista de países exceptuados, el «Six-Month Club»)
no estaba en ningún lado: el sistema la había resuelto bien, pero por descarte,
y no lo decía.

## Lo que cambia

1. **Cada renglón explica.** Se separa en **qué pide el país**, **qué tiene el
   pasajero** y **qué hacer**, con las fechas, el número de documento y la regla
   completa (la general y la excepción que le toca a esta nacionalidad).
2. **El trámite.** Donde el requisito pide hacer algo (visa, autorización
   electrónica, tarjeta turista, vacuna, registro…), el renglón dice qué es,
   dónde y cómo se pide, si hay entrevista y qué papeles se presentan.
3. **Una fuente nueva** para lo que Wikipedia no trae: búsqueda en páginas
   oficiales.
4. **Recomendación de renovar el pasaporte** cuando vence dentro de los 6 meses
   posteriores al regreso.
5. **El aviso al cliente** gana el «qué hacer» y el trámite.

**No entran ahora** (lo dijo la agencia): costos de los trámites y tiempos de
espera de turnos.

## Lo que no se negocia

Los principios del diseño original siguen igual, y estos se suman:

- **Nada de lo que se muestra se inventa.** Cada frase del detalle que sale de
  la búsqueda lleva su enlace y una cita textual que el código comprueba contra
  el texto real de la página. Si la cita no está, la frase no se muestra.
- **La búsqueda suma detalle y sirve de control; nunca afloja un requisito por
  su cuenta.** No completa campos de la regla. Si contradice lo que dicen
  Wikipedia y Passport Index, el campo pasa a «verificar» con el motivo «las
  fuentes no coinciden».
- **El porqué con los datos del pasajero no depende de la búsqueda.** Se arma
  con la regla y los datos cargados. Sin la búsqueda, el renglón está completo
  igual, solo sin el trámite ni la regla explicada.

---

## Parte 1: el renglón explicado (evaluación)

### La forma del renglón

El renglón tiene un **título** y un **detalle**. El título es el nombre del
requisito, como en los ejemplos: «Documento», «Vigencia del pasaporte», «Visa»,
«Estadía», «Vacuna: fiebre amarilla», «Condición: registro», «Tránsito»,
«Autorización de menor». Las frases sueltas de hoy desaparecen: su contenido se
reparte entre «pide», «tiene» y «hacer». `Text` (y la columna `text`) pasa a
guardar el título; `ClientText` sigue igual como mecanismo del aviso. *(Decisión
de quien escribe el diseño, siguiendo los ejemplos que aprobó la agencia.)*

`evaluacion.Requirement` suma `Detail`:

| Campo | Qué es |
|---|---|
| `pide` | lo que pide el país, en una o más frases: la regla tal como la entiende el sistema y, si la búsqueda la trajo, la explicación (la general y la excepción) |
| `tiene` | lo que tiene el pasajero que pesa en este requisito: documento con su tipo, país, número, emisión y vencimiento; permisos con su vencimiento; fechas del viaje |
| `hacer` | el próximo paso, solo en «falta» y «verificar». Dice qué cargar, qué tramitar o qué revisar |
| `tramite` | las frases del trámite que trajo la búsqueda (ver Parte 2), solo donde hay trámite |
| `fuentes` | todas las fuentes del renglón: las de la regla y las de la búsqueda, cada una con su cita y su fecha |

Cada frase de `pide` y `tramite` lleva su propia fuente (una frase sale de una
página y otra de otra). Las de `tiene` y `hacer` las arma el código y no llevan
fuente.

`evaluacion.Document` suma `Number`, que hoy no llega al motor.

### Ejemplo: argentino a Estados Unidos

> **Cumple · Documento**
> *Pide:* Estados Unidos pide pasaporte: el DNI argentino no alcanza.
> *Tiene:* pasaporte argentino AAB123456, emitido el 12/05/2021, vence el
> 12/05/2031.

> **Falta · Vigencia del pasaporte**
> *Pide:* En general, Estados Unidos pide que el pasaporte venza al menos 6
> meses después de salir del país. La Argentina está exceptuada (lista del
> «Six-Month Club»): alcanza con que esté vigente toda la estadía.
> *Tiene:* el pasaporte vence el 12/05/2031. El viaje no tiene fechas cargadas.
> *Hacer:* cargar las fechas del viaje. Si vuelve antes del 12/05/2031, cumple.

> **Falta · Visa**
> *Pide:* Estados Unidos pide visa consular a los argentinos.
> *Trámite:* Para turismo es la visa B1/B2. Se completa el formulario DS-160
> online, se paga y se hace una entrevista en la embajada en Buenos Aires…
> *Tiene:* no tiene visa de Estados Unidos cargada.
> *Hacer:* si ya la tiene, cargarla en Pasajeros con su vencimiento; si no,
> tramitarla.
> *Fuentes:* Wikipedia (revisión del 18/10/2026); travel.state.gov, leído el
> 21/10/2026.

### Qué dice cada requisito

El título conserva el estado de hoy; lo que se agrega es el detalle. La
redacción final es del builder. Lo que tiene que estar:

| Requisito | Pide | Tiene | Hacer |
|---|---|---|---|
| Documento | si alcanza el DNI o hace falta pasaporte, y por qué (el país está o no en la lista de los que aceptan DNI) | el documento con el que viaja, con número y vencimiento | en «falta»: viajar con pasaporte, o cargarlo si ya lo tiene |
| Vigencia | la cantidad y desde cuándo se cuenta, dicha en palabras («vigente toda la estadía», «6 meses después de salir del espacio Schengen») y la fecha límite calculada si hay fechas | vencimiento del documento; fechas del viaje o que faltan | sin fechas: cargarlas y la fecha hasta la que cumple igual («si vuelve antes del…»); vencido para el viaje: renovar |
| Antigüedad | la antigüedad máxima | fecha de emisión | cargar la fecha o renovar |
| Visa | la categoría, las exenciones que conoce la regla y cuál se aplicó | los permisos de ese país (y el que habilitó la exención) con su vencimiento | cargar el permiso si ya lo tiene, o tramitarlo; si vence antes del viaje, renovarlo |
| Estadía | los días permitidos sin visa | los días que se queda, con las fechas | acortar la estadía o tramitar visa |
| Vacunas | la vacuna y a quién se le pide | — | conseguir el certificado; si es por país de riesgo, revisar el país de salida |
| Tránsito | si el país pide visa a esta nacionalidad | — | revisar si la escala necesita visa de tránsito |
| Condiciones | la condición completa | lo que el viaje ya resuelve (regreso cargado) | lo que hay que llevar o hacer |
| Menor | la regla argentina de salida de menores | la edad a la fecha de salida | conseguir la autorización o confirmar que viaja con ambos padres |

Las líneas de «verificar» dicen en `pide` qué no se sabe y por qué (el motivo de
hoy) y en `hacer` qué tiene que revisar la agencia.

### Recomendación de renovar el pasaporte

Un **aviso** por pasajero (no por país) cuando el documento de viaje vence
**antes de los 6 meses posteriores al regreso** del viaje: «El pasaporte vence
el 12/02/2027, a menos de 6 meses de la vuelta (15/10/2026). Recomendamos
renovarlo: algunas aerolíneas lo piden igual».

- Es un estado nuevo, **`aviso`**: no es cumple, falta ni verificar. **No pesa
  en los controles** (no abre «Requisitos migratorios» ni «Vencimiento de DNI o
  pasaporte») y no se confirma.
- Va al aviso del cliente como recomendación, aunque todo lo demás esté en
  orden: «está todo en orden con la documentación. Te recomendamos renovar el
  pasaporte de María, que vence el 12/02/2027».
- Sin fecha de regreso o sin vencimiento cargado, no se calcula (lo que falta ya
  lo dicen los otros renglones).
- Si el documento ya no cumple la vigencia de algún país, no se agrega: ese
  renglón ya está en «falta».
- Los 6 meses son una constante del código, con su motivo escrito al lado.

### Vista previa (sin pasajeros)

`PreviewLine` suma `pide` y `tramite` con sus fuentes. No tiene `tiene` ni
`hacer`.

### Dónde se guarda

`trip_requirements` suma una columna `detail` (jsonb) con el detalle del
renglón. Se recalcula igual que hoy. Los controles siguen leyendo solo el
estado; el estado `aviso` se agrega a lo que cuenta como resuelto.

---

## Parte 2: la búsqueda en páginas oficiales (módulo `requisitos`)

### Qué busca

Para cada regla (nacionalidad + país), Claude busca en la web y arma **el
detalle**: frases en español, cada una con su cita textual y su enlace,
agrupadas por requisito:

| Grupo | Qué trae |
|---|---|
| `documento` | qué documento se pide y por qué |
| `visa` | la explicación de la categoría y, si hay trámite, `tramite`: qué tipo de visa o autorización es (para turismo), dónde y cómo se pide, si hay entrevista y qué papeles se presentan |
| `vigencia` | la regla de vigencia completa: la general y la excepción que corresponde a esta nacionalidad |
| `antiguedad` | si el país pide un pasaporte emitido hace menos de X años |
| `estadia` | cuánto se puede quedar y cómo se cuenta («90 días en 180») |
| `vacunas` | qué vacuna, a quién, y cómo se acredita |
| `transito` | si una escala pide visa de tránsito |
| `condiciones` | por cada condición de la regla, el detalle de cómo se cumple (dónde registrarse, qué formulario…) |

Además devuelve, para el control (ver «Contra las otras fuentes»), lo que dicen
las páginas sobre cuatro valores: la categoría de visa, si alcanza el DNI, la
vigencia y la estadía. Cada uno con su cita, o nulo si las páginas no lo dicen.

**Sin costos ni tiempos de espera.** El prompt lo excluye.

### Qué páginas

- **Primero las oficiales:** gobierno del país de destino (migraciones,
  cancillería), sus embajadas y consulados, y organismos como la Unión Europea.
  Después el gobierno del país de la nacionalidad (las cancillerías publican
  guías por destino).
- **Nunca:** agencias que venden visas (iVisa, VisaHQ y parecidas), blogs y
  foros. Se bloquean por dominio las que se conozcan, y el prompt lo dice.
  Wikipedia queda fuera porque ya es una fuente.
- La ficha muestra el dominio de cada fuente, para que quien lee vea de dónde
  salió.

### Cómo se comprueba

- **La cita tiene que estar en la página.** El código compara cada cita contra
  el texto de la página citada (normalizando espacios y mayúsculas). El texto
  tiene que ser el de la página, no lo que diga el modelo: o el documento que
  devolvió la herramienta de lectura, completo, o una descarga propia de esa
  URL. Si la cita no aparece, la frase se descarta.
- **Los números de la frase tienen que estar en la cita.** «Hay que
  registrarse dentro de las 24 horas» necesita una cita con el 24. Si no, la
  frase se descarta. Así una cifra mal traducida no llega a la ficha.
- **Solo enlaces que se leyeron.** Una frase que cita una URL que no se leyó en
  esa búsqueda se descarta.
- El prompt aclara que el contenido de las páginas es un dato y que no hay que
  seguir instrucciones que aparezcan adentro, como hoy con Wikipedia.

### Contra las otras fuentes

Los cuatro valores de control se comparan en `domain.Combine` con los de la
regla:

- **Coinciden, o la búsqueda no dice nada:** la regla queda como está y las
  citas de la búsqueda se suman a las fuentes del campo.
- **No coinciden:** el campo pasa a «verificar» con el motivo «las fuentes no
  coinciden» y el conflicto se registra, como hoy con Passport Index. La ficha
  muestra qué dijo cada fuente.
- **Un campo en «verificar» o sin valor sigue así**, aunque la búsqueda lo
  diga. La búsqueda no completa campos.
- **Solo se compara contra valores con fuente.** Un valor que la regla pone por
  defecto, sin fuente (por ejemplo `documento = pasaporte` cuando Wikipedia no
  dice nada, o la estadía vacía), no cuenta como «lo que dice la regla». Ahí la
  búsqueda solo puede endurecer: si dice algo **más exigente** que el valor por
  defecto (una estadía máxima de 30 días donde no había ninguna, pasaporte
  donde había DNI sin fuente), el campo pasa a «verificar»; si dice algo **más
  permisivo** (que alcanza el DNI donde por defecto se pide pasaporte), se
  ignora para el valor y no se muestra.
- **Los valores de control valen para los campos que la búsqueda vio.** La
  búsqueda guarda la huella de los campos de la regla contra los que se hizo. Si
  después la pasada semanal cambia un campo (por ejemplo, la visa pasa de
  consular a electrónica), los valores de control de ese campo se ignoran en
  `Combine`, el detalle de ese grupo se oculta (la ficha dice «buscando el
  detalle del trámite…») y la búsqueda se vuelve a encolar enseguida, sin
  esperar al mes. Los campos cambian pocas veces, así que esto agrega pocas
  búsquedas. Lo mismo con las condiciones: si cambia la lista, se oculta su
  detalle hasta la búsqueda nueva.
- La vigencia se compara ya resuelta para la nacionalidad: «6 meses, salvo el
  Six-Month Club» para un argentino es «toda la estadía», y coincide con
  Wikipedia.
- Las constantes del código siguen ganando en sus campos (Schengen, DNI
  argentino); la búsqueda no las pone en «verificar», solo agrega detalle.

### Cuándo corre

- **Primera vez:** cuando la regla queda vigente por primera vez, en el mismo
  proceso de fondo y **después** de la extracción de Wikipedia. La regla no
  espera a la búsqueda: el renglón se muestra con su porqué y dice «buscando el
  detalle del trámite…» hasta que llega.
- **Solo donde hay un trámite que explicar:** visa consular, visa electrónica,
  autorización electrónica o visa a la llegada. Donde no hace falta visa, o la
  visa está en «verificar», no se busca: alcanza con lo que arma el sistema.
  *(Corrección después de medir el costo real, ver «Modelo y costo».)*
- **Cada tres meses** por regla, aparte de la pasada semanal de Wikipedia. Las
  páginas oficiales cambian poco y la búsqueda es lo más caro.
- **Si falla** (sin clave, error, ninguna frase pasa la comprobación): se queda
  el detalle anterior. Si nunca hubo detalle, el renglón dice «no se encontró el
  detalle en páginas oficiales» y se reintenta al día siguiente, hasta tres
  veces seguidas; después vuelve a la frecuencia de tres meses. Así una página que no
  se deja leer no cuesta una búsqueda por día para siempre.
- Un detalle nuevo que cambia el texto re-evalúa los viajes abiertos que usan la
  regla (por el mismo gancho de hoy), para que la ficha se actualice. Un detalle
  que no cambia los campos de la regla no sube su versión y no invalida
  confirmaciones.

### Dónde se guarda

En `entry_rules`: `details` (jsonb), `details_checked_at` y `details_status`
(`buscando`, `listo`, `sin_datos`). Cada frase guarda la URL, la cita y la fecha
en que se leyó la página. El historial de versiones guarda también el detalle.

El consumo se registra como hoy, sumando la cantidad de búsquedas web.

### Modelo y costo

*(Corregido después de medir con fuentes reales: la estimación original, de
USD 0,30 a 0,60 con Opus, quedó corta por mucho.)*

Medido con argentino a Estados Unidos y a España:

| Modelo | Costo por búsqueda | Resultado |
|---|---|---|
| Opus 5.5, sin caché | USD 0,85 a 2,75 | bueno, caro |
| Sonnet 5.5, con caché y topes | USD 0,31 | bueno |
| Haiku 5.5, con caché y topes | USD 0,04 a 0,07 | irregular: una de dos corridas no trajo nada |

Casi todo el costo es el texto leído, que se vuelve a cobrar en cada vuelta de
la búsqueda, no las búsquedas en sí (USD 10 cada mil). Por eso:

- **Sonnet 5.5** (`claude-sonnet-5-5`) por defecto, configurable con
  `REQUISITOS_MODELO_BUSQUEDA`. Ninguno de los tres resolvió solo la excepción
  de los 6 meses de Estados Unidos (la lista está en un PDF): es una regla fija
  del código, como Schengen (ver «La regla de los 6 meses de Estados Unidos»).
- **Caché** en todos los pedidos: lo ya leído se cobra al 5%.
- **Topes:** 4 búsquedas, 8 lecturas y 12.000 tokens por página.
- **La búsqueda va detrás de un interruptor, apagado por defecto**
  (`REQUISITOS_BUSQUEDA_OFICIAL`). Sin encenderlo no gasta nada.
- El consumo se guarda con lo leído y escrito en la caché, para calcular el
  costo real.

Con unos 10 destinos con visa, una búsqueda cada tres meses son unos USD 12 por
año.

### La regla de los 6 meses de Estados Unidos

Estados Unidos pide que el pasaporte venza al menos 6 meses después de la
estadía, salvo a los países de la lista del Carrier Liaison Program de la CBP
(publicación 5303-1225, 18/12/2025), que solo necesitan el pasaporte vigente
durante la estadía. La Argentina está en la lista. Es una constante del código
con la lista y su fuente; el renglón explica la regla general y la excepción.

**Lo que queda para el builder:** el prompt; si la búsqueda y el armado del
detalle van en una sola llamada con salida estructurada o en dos (buscar y leer;
después armar sobre los textos leídos); cómo se obtiene el texto de la página
para comprobar las citas; la lista de dominios bloqueados; el tope de búsquedas.

---

## Pantalla

Cada renglón de la pestaña «Requisitos»:

```
[Falta]  Vigencia del pasaporte
         Pide   En general, Estados Unidos pide que el pasaporte venza al
                menos 6 meses después de salir del país. La Argentina está
                exceptuada: alcanza con que esté vigente toda la estadía.
         Tiene  Pasaporte argentino AAB123456, vence el 12/05/2031.
                El viaje no tiene fechas cargadas.
         Hacer  Cargar las fechas del viaje. Si vuelve antes del
                12/05/2031, cumple.
         Fuentes: Wikipedia · travel.state.gov (21/10/2026)
```

- **El detalle se ve siempre**, sin «ver más»: es lo que la agencia lee para
  contestarle al cliente.
- El título es la frase corta en negrita; debajo, «Pide», «Trámite», «Tiene» y
  «Hacer» como etiquetas chicas a la izquierda. Una parte vacía no se muestra.
- **Fuentes:** todas las del renglón, con el nombre (Wikipedia, Passport Index,
  norma oficial) o el dominio de la página oficial, y la fecha en que se leyó.
  La cita se ve al pasar el mouse.
- El aviso de renovar el pasaporte va arriba de los países del pasajero, con su
  propio estilo (no rojo).
- Mientras la búsqueda no terminó: «Buscando el detalle del trámite…» en el
  lugar del trámite.
- Vale lo de siempre: palabras y no códigos, y nada en color sin una frase que
  diga por qué. La pestaña vive al lado del chat y es angosta: las etiquetas no
  pueden comerse el ancho.

## El aviso al cliente

Cada ítem del mensaje suma lo que hay que hacer y, si hay trámite, una frase
corta de cómo se pide:

> • Beto: para Estados Unidos necesita la visa de turismo (B1/B2). Se pide
>   completando el formulario DS-160 online y con una entrevista en la embajada.

La frase del trámite sale del detalle de la búsqueda; si no hay, queda como
hoy. Las recomendaciones (renovar el pasaporte) van al final, aunque esté todo en
orden. Lo que está en «verificar» sigue sin ir al mensaje.

---

## Decisiones tomadas sin consultar

- la búsqueda no completa campos de la regla, solo agrega detalle y controla;
- el título del renglón es el nombre del requisito y las frases de hoy se
  reparten en «pide», «tiene» y «hacer»;
- contra un valor por defecto sin fuente, la búsqueda solo puede endurecer
  (pasa a «verificar»); lo más permisivo se ignora;
- si la pasada semanal cambia un campo, el detalle y el control de ese campo se
  dejan de usar y la búsqueda se repite enseguida;
- sin resultados, tres reintentos diarios y después mensual;
- el detalle se guarda aparte de los campos, para que un cambio de redacción no
  invalide confirmaciones;
- los números de cada frase tienen que estar en su cita;
- la recomendación de renovar es por pasajero, con un estado nuevo `aviso` que
  no pesa en los controles;
- la recomendación se calcula contra la fecha de regreso del viaje;
- búsqueda cada tres meses y solo en destinos con trámite, separada de la
  pasada semanal de Wikipedia;
- Sonnet 5.5 para la búsqueda, con el modelo configurable, detrás de un
  interruptor apagado por defecto;
- la regla de los 6 meses de Estados Unidos como constante;
- el detalle se ve siempre, sin desplegar;
- el número de documento se muestra completo en la ficha (es una herramienta
  interna de la agencia; no va en el aviso al cliente).

## Riesgos

- **Una página oficial mal leída.** Una cita real puede interpretarse mal (por
  ejemplo, aplicar a un argentino una excepción que es para otro país). Mitigan:
  la búsqueda no afloja campos; si contradice, el campo pasa a «verificar»; y
  cada frase muestra su fuente y su cita.
- **Páginas que no se dejan leer** (con JavaScript o que bloquean robots): sus
  citas no se pueden comprobar y las frases se descartan. El renglón queda con
  menos detalle, pero con el porqué de la regla.
- **Costo:** se mide con el consumo guardado; el tope de búsquedas por regla y la
  frecuencia mensual lo acotan.

## Verificación

Pruebas en Go del motor (puras):

- cada requisito arma `pide`, `tiene` y `hacer` con los datos del pasajero (el
  número y las fechas del documento aparecen);
- vigencia sin fechas del viaje dice hasta qué fecha cumpliría;
- un pasaporte que vence 4 meses después del regreso da el aviso de renovar; uno
  que vence 7 meses después, no; sin fecha de regreso, no;
- el aviso no pesa en los controles y va al mensaje del cliente aunque todo esté
  en orden;
- un renglón sin detalle de búsqueda está completo igual y no tiene trámite.

Pruebas en Go de la búsqueda y la combinación:

- una frase con una cita que no está en la página se descarta;
- una frase con un número que no está en su cita se descarta;
- una frase que cita una URL que no se leyó se descarta;
- la búsqueda dice «no requiere visa» y Wikipedia «visa required» → «verificar»
  y el conflicto se registra;
- la búsqueda coincide → el campo sigue igual y suma las citas;
- un campo en «verificar» no se completa con la búsqueda;
- contra un valor por defecto sin fuente: la búsqueda más exigente → «verificar»;
  la más permisiva → se ignora;
- un campo que cambió después de la búsqueda ignora sus valores de control,
  oculta su detalle y encola una búsqueda nueva;
- tres búsquedas fallidas seguidas pasan a frecuencia mensual;
- un cambio en el detalle no sube la versión de los campos ni invalida
  confirmaciones, y sí re-evalúa los viajes;
- sin clave, el detalle queda `sin_datos` y la regla sigue funcionando.

Pruebas de componente:

- el renglón muestra «Pide», «Tiene» y «Hacer», y oculta las partes vacías;
- las fuentes muestran el dominio y la fecha;
- «Buscando el detalle del trámite…» mientras la búsqueda está en curso;
- el aviso de renovar se ve arriba de los países del pasajero.

Prueba manual con fuentes reales: pasaporte argentino a Estados Unidos, Canadá,
España, Cuba y Japón. Comparar el detalle con las páginas oficiales y revisar que
Estados Unidos explique el Six-Month Club.
