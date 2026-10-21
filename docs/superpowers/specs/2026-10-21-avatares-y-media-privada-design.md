# Avatares temporales y media privada

Fecha: 2026-10-21

## Intención

Mostrar en la bandeja la foto de perfil que WhatsApp permite ver, sin convertir
esas fotos en archivos permanentes del CRM. Mantener, en cambio, una copia
durable de las imágenes y documentos enviados dentro de las conversaciones,
porque los enlaces de WhatsApp no son un archivo estable.

## Decisión

Los dos tipos de imagen tendrán ciclos de vida distintos:

- **Foto de perfil:** se consulta bajo demanda a Evolution Go, se copia sólo a
  memoria por seis horas y nunca se guarda en Postgres, R2 ni el volumen local.
  Si WhatsApp no permite verla, no existe o Evolution tarda demasiado, el panel
  conserva las iniciales actuales.
- **Adjunto de una conversación:** se conserva con el pipeline de media ya
  existente. Evolution entrega el binario descifrado en el webhook, el backend
  lo sube al `blobs.Store` y la interfaz lo abre mediante una URL firmada de
  vida corta. Con credenciales completas el store es R2; sin ellas es el volumen
  Docker local.

No se habilitará un bucket público ni se enviará al navegador la API key de
Evolution. Las credenciales de R2 tampoco se guardarán en el repositorio.

## Flujo del avatar

1. Las respuestas de bandeja y conversación incluyen una ruta relativa y
   autenticada: `/api/mirror/conversations/{id}/avatar`.
2. `ContactAvatar` pide esa ruta con el mismo JWT que usa el resto de la API y
   crea un `blob:` URL local mientras el componente está montado. Un `img` no
   recibirá el JWT como query param.
3. El backend resuelve desde la conversación el `evolution_name` de la línea y
   el `remote_jid`, y llama `POST /user/avatar` con el token derivado de esa
   instancia y `preview: true`.
4. El backend sólo acepta URLs HTTPS de hosts de WhatsApp, incluida cada URL de
   una eventual redirección, descarga como máximo 2 MiB y exige un
   `Content-Type` de imagen.
5. El resultado se guarda en una caché de memoria privada por seis horas. La
   caché tendrá un techo total de 32 MiB; las entradas vencidas y las más
   antiguas se desalojan antes de superar ese techo.
6. Las cargas simultáneas de una misma conversación se agrupan en una sola
   consulta. Las ausencias conocidas se guardan también en memoria durante
   quince minutos; un error de base de datos sigue siendo un error interno y no
   se disfraza como ausencia de foto.
7. Antes de servir una entrada positiva de caché se vuelve a resolver la
   conversación. Así no se sirve durante seis horas la foto de una conversación
   borrada o una línea desactivada.
8. La respuesta usa `Cache-Control: private, max-age=21600`. Las ausencias se
   responden como 404 con caché privada de quince minutos para que la UI caiga a
   iniciales sin martillar Evolution.

El tiempo máximo del camino a Evolution será corto para una imagen decorativa:
ocho segundos. Una foto nunca debe bloquear la bandeja ni aparecer como un
error visible de la aplicación.

## Compatibilidad

Evolution Go 0.7.2 devuelve la foto en `data.URL`. Su endpoint tiene fallos
conocidos de timeout para algunos JID/LID, así que el contrato del CRM trata la
foto como opcional. Los grupos usan el mismo flujo y su `remote_jid` de grupo.

El campo existente `contacts.profile_pic_url` deja de ser una fuente para la
interfaz: puede contener un enlace vencido y no expresa desde qué línea se pudo
ver la foto. No se elimina la columna en este cambio para evitar una migración
destructiva innecesaria.

## Media de conversaciones y R2

El pipeline actual ya cumple la persistencia requerida:

- `WEBHOOK_FILES=true` hace que Evolution descargue y descifre el adjunto;
- el webhook acepta el `base64` resultante;
- el worker de media limita, hashea y sube el binario;
- `media_assets` borra la fuente transitoria al marcar el objeto como guardado;
- `GET /api/mirror/media/{id}` exige sesión y devuelve una URL firmada;
- el bucket R2 permanece privado.

El barrido de recuperación incluirá también trabajos que lleven más de diez
minutos en `processing`. Hoy un reinicio entre `MarkProcessing` y `MarkStored`
puede dejarlos trabados para siempre; verificar durabilidad exige cerrar ese
hueco.

La decisión de retención es redactar las fuentes binarias del webhook una vez
procesado. Al ejecutar `MarkProcessed`, se eliminan únicamente
`data.Message.base64` y `data.Message.mediaUrl` de `webhook_events.payload`; el
resto de los metadatos queda disponible para auditoría. La copia transitoria en
`media_assets.source_data`/`source_url` se conserva sólo hasta que el objeto se
almacena correctamente y luego también se limpia.

Para activar R2 se deben definir juntas `R2_ACCOUNT_ID`, `R2_ACCESS_KEY_ID`,
`R2_SECRET_ACCESS_KEY` y `R2_BUCKET`. El entorno actual no las tiene cargadas,
por lo que hoy usa el volumen `blobs`. La activación real queda condicionada a
recibir esas credenciales; no bloquea el desarrollo ni las pruebas del avatar.
Al cambiar de local a R2, los objetos que ya están en el volumen no se copian
solos y dejan de ser visibles para el backend configurado con R2. No se cambiará
el store hasta decidir y ejecutar una copia de esos objetos o aceptar
explícitamente que queden sólo en el entorno local anterior.

## Fuera de alcance

- Guardar un historial de fotos de perfil.
- Sincronizar todos los avatares en segundo plano.
- Publicar R2 o exponer URLs permanentes.
- Cambiar Evolution Go o resolver dentro de este trabajo su bug de conexiones.
- Construir una migración automática y permanente entre stores; una copia
  puntual previa a activar R2 se planificará cuando existan las credenciales.

## Criterios de aceptación

- Una conversación con foto visible muestra esa foto en lista, hilo y detalle.
- Una foto oculta, ausente, inválida o lenta conserva las iniciales sin romper
  la pantalla.
- El JWT no aparece en el DOM, el `src` final ni la URL solicitada como query.
- La segunda petición dentro de seis horas usa la caché del backend.
- Ningún avatar se escribe en disco, Postgres o R2.
- Los adjuntos siguen funcionando en almacenamiento local y cambian a R2 al
  aportar las cuatro variables, conservando URLs firmadas.
