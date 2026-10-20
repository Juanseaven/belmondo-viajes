# Ficha editable de operación — diseño

## Objetivo

Reemplazar el alta bloqueante de destino y fechas por una ficha de operación que esté visible desde que se abre una conversación y permita cargar viaje, pasajeros o pagos en cualquier orden.

## Flujo aprobado

- Sin viaje persistido, el panel muestra `Operación nueva`, las pestañas Resumen, Pasajeros, Pagos y Controles, y valores pendientes como `Sin completar`.
- Abrir una conversación no escribe en la base.
- La primera acción de guardado crea el viaje borrador y guarda el dato elegido. Esto aplica aunque el primer dato sea un pasajero o un pago.
- El borrador recibe número visible al crearse. Destino, salida y regreso dejan de ser obligatorios y pueden completarse después.
- Los controles se materializan al crear el borrador. Los vencimientos dependientes de salida o regreso quedan sin fecha hasta que exista la fecha correspondiente y se recalculan al editarla.
- No se simulan secciones que todavía no existen en el dominio (Servicios, Documentos, Actividad o nota interna).

## Interfaz

El panel conserva el lenguaje compacto del CRM: encabezado con número/estado, pestañas accesibles y secciones densas con edición inline. En Resumen, destino y fechas se editan juntos pero ninguno es obligatorio. Pasajeros y Pagos reutilizan sus formularios actuales. Controles explica que se generan al primer guardado cuando todavía no existe el borrador.

## Persistencia y errores

La base permite `NULL` en destino y fechas. La API acepta altas parciales y sigue rechazando fechas mal formadas o un regreso anterior a la salida cuando ambas están presentes. El frontend espera a que el viaje se cree antes de guardar un pasajero o pago y recarga la ficha al terminar. Los formularios conservan un estado de guardado y muestran el error sin cerrar ni perder los datos escritos.

