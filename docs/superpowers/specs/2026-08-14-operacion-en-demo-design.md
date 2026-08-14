# Diseño de la operación en la demo comercial

## Objetivo

Incorporar la **operación** —el expediente de un viaje vendido— a la demo
comercial de `/mirror?demo=1`, de modo que la pantalla muestre no solo la
conversación sino el estado administrativo del viaje que se está negociando.

El trabajo reemplaza dos planillas en papel de la agencia: la de carga de
reserva y la de control de reserva, incluida su tabla de tarifas. No las
digitaliza como listas de casilleros; las convierte en datos y en controles con
evidencia.

Todo ocurre en el frontend de demostración con datos fijos. No hay endpoints,
persistencia ni cambios de backend.

## Principio operativo

Una planilla digital de diecisiete casilleros se convierte en teatro: alguien
tilda las diecisiete sin mirar y el error que la hoja quería evitar ocurre
igual, ahora con un registro que afirma lo contrario. El tilde es una
declaración.

El diseño evita ese resultado con una regla: **el estado de un control es una
consecuencia, no una declaración.** Cuando la acción se puede ejecutar desde el
sistema —enviar el voucher, avisar por visa, informar el monto no
reembolsable—, el control se cierra al ejecutarla y queda enlazado al mensaje
que lo prueba. Nadie tilda nada.

Cuando la acción no se puede ejecutar desde el sistema —el régimen de comidas,
el orden de los nombres— el control se confirma a mano, pero registrando quién
lo confirmó y cuándo. Eso es lo que la planilla en papel buscaba y no podía dar.

## El modelo

### Operación

La operación cuelga del contacto, no al revés. Un contacto puede tener varias a
lo largo del tiempo; la conversación apunta a la que está activa.

Tiene número visible, estado (`cotizada`, `confirmada`, `en viaje`, `cerrada`),
destino, fecha de salida, fecha de regreso, vendedor —una persona, no la línea
de WhatsApp— y fecha de creación con su autor.

De ella cuelgan pasajeros, servicios, pagos, documentos y controles.

### Pasajeros

Absorbe de las planillas: nombres completos, apellidos, orden de los nombres y
apellidos, nombre y apellido del titular de la reserva, y vencimiento de
DNI o pasaporte.

Cada pasajero registra su nombre tal como figura en el documento, si es el
titular, si es adulto o menor, y el tipo, número y vencimiento de su documento.

El resumen actual —«2 ad + 1 men»— deja de ser un conteo y pasa a ser una lista
de personas. El ítem resaltado en la planilla de control, el nombre del titular,
vive acá.

### Servicios

Absorbe de las planillas: hotel correcto, régimen de comidas, control de
traslados, equipaje incluido, asistencia médica, Flight Plus y vencimiento del
alojamiento.

Cada servicio registra su tipo (aéreo, hotel, traslado, asistencia, excursión),
su detalle, el proveedor, las fechas cuando corresponde, su estado (`cotizado`,
`reservado`, `emitido`) y **su valor**.

Ese valor es la pieza que disuelve la segunda tabla de la planilla de control.
Valor de vuelos más gasto de emisión, transfer, hotel 1, hotel 2, asistencia,
merch más fee y Flight Plus son los valores de los servicios de la operación. El
«control de liquidación» —verificar que los montos cotizados coincidan con la
liquidación— se vuelve la suma de los servicios comparada contra el total. Lo
calcula el sistema y no hay nada que controlar a mano.

### Pagos

Absorbe de las planillas: valor en USD, valor en pesos, cambio del día, método
de pago y, cuando el pago es con tarjeta, el comprobante del pasajero y el
comprobante de carga en el portal.

Registra además seña, saldo y fecha de vencimiento de la seña.

### Documentos

Documentos de identidad, comprobantes de pago y vouchers.

Cada documento conserva su procedencia: llegó por WhatsApp, con enlace al
mensaje y su hora, o lo subió una persona identificada. Los documentos que hoy
se pierden en el scroll del hilo —el comprobante de transferencia, el DNI de los
menores— quedan reunidos.

### Controles

Un único objeto cubre lo que en la imagen de referencia aparecía como tres
listas distintas —controles, tareas y documentos— diciendo el mismo hecho tres
veces. Tres objetos separados se desincronizan y el sistema termina mintiendo.

Un control registra título, etapa (carga o control), criticidad, estado,
responsable, vencimiento y **la forma en que se cierra**:

- **Por acción.** El sistema ejecuta la acción y el control queda cerrado con la
  evidencia enlazada: voucher enviado, aviso de visa o vacuna, términos y
  condiciones aceptados, monto no reembolsable informado, comprobante recibido.
- **Manual.** El control se confirma explícitamente y queda registrado quién lo
  confirmó y cuándo: régimen de comidas, orden de los nombres, equipaje
  incluido, hotel reservado igual al cotizado.

La tarea es un control con responsable y fecha. El documento es la evidencia de
un control. Un objeto, tres formas de mirarlo.

## Qué se ve en pantalla

### Fila de la bandeja

Cada fila muestra a qué operación pertenece la conversación, con su número y
destino.

El chip de estado deja de estar escrito a mano en los datos de demostración y
pasa a derivarse de los controles: muestra el control crítico abierto más
próximo a vencer; si no hay ninguno crítico, el control abierto más próximo a
vencer; y si no hay controles abiertos, el estado de la operación. Esa columna
es, en el fondo, lo que la planilla de control quería ser: lo que un supervisor
necesita ver sin abrir la conversación.

### Banner del hilo

Sobre los mensajes, una franja con la operación activa —número, destino, fechas,
pasajeros y total— y un acceso a la operación completa. Conecta las dos mitades
de la pantalla.

### Panel derecho

Reemplaza las pestañas actuales de ficha y actividad por: **Resumen**,
**Pasajeros**, **Servicios**, **Pagos**, **Documentos** y **Actividad**.

Los datos del contacto —teléfono, mail, cliente desde, atendido por— quedan como
un bloque dentro de Resumen.

El Resumen muestra destino, fechas, pasajeros, total en ambas monedas, vendedor
y estado de pago con seña, saldo y vencimiento. Debajo, los controles críticos
abiertos.

**No hay barra de progreso ni porcentaje de controles completados.** Una barra
al 64 % le pide a alguien que la lleve al 100 %, y el camino más corto para eso
es tildar; sería el mismo teatro con un número encima. Además el promedio
esconde lo que importa: trece de catorce con el pasaporte vencido es más grave
que nueve de catorce con todo lo crítico resuelto. Se muestran los controles
críticos abiertos y, si no hay ninguno, se dice eso.

## Cambios al panel actual

Se agrega el **mail del contacto**. Las planillas lo piden de forma explícita
—«tel y mail del pax cargados en reserva», «aviso vía mensaje y mail al pax»,
«se envió por mail el monto no reembolsable»— y hoy no existe en la ficha.

Se quitan las **etiquetas**. Las tres actuales son campos disfrazados:
«Recurrente» se deduce del historial, «Paquete nacional» es el tipo de producto
y «Pago en pesos» es el método de pago. Un cajón de etiquetas libres termina
siendo donde se guarda lo que no se supo modelar.

Se quita **Ciudad**: WhatsApp no la provee, nadie la mantiene y ningún paso del
proceso administrativo la usa.

## Alcance

Entra en este trabajo:

- la operación como entidad en los datos de demostración;
- el banner de operación activa en el hilo;
- el panel derecho con sus seis pestañas;
- la referencia a la operación en la fila de la bandeja;
- el chip de estado derivado del control crítico abierto.

Queda afuera:

- el rail de navegación de la imagen de referencia;
- la página de listado de operaciones;
- las pantallas de tareas, calendario, reportes y plantillas;
- la página de clientes;
- toda persistencia, endpoint o cambio de backend.

## Datos de demostración

Tres operaciones con datos completos —Marina Ferreyra a Iguazú, Grupo Rossi a
Bariloche y Diego Balbuena a Cancún— siguiendo el criterio que ya rige los
perfiles actuales. El resto de las conversaciones tiene operación mínima o
ninguna, como ocurre hoy con las fichas.

Las tres operaciones completas cubren entre sí los estados relevantes: una
cotizada sin seña confirmada, una confirmada con controles pendientes y una con
un control crítico abierto.

Las fechas se mantienen en 2026, coherentes con los datos de demostración
existentes.

## Verificación

Pruebas de componente sobre el panel de operación, la derivación del chip de
estado a partir de los controles críticos y el cierre de un control por acción
con su evidencia enlazada.

Al cierre: `npm test`, `npm run typecheck` y `npm run build`, más inspección
visual de `/mirror?demo=1` sin scroll horizontal de página.

Queda pendiente de arreglo un defecto previo: `MirrorScreen.demo.test.tsx` falla
con `document is not defined` por falta de entorno jsdom en la corrida.
