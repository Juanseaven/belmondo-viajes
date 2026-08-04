import { mirror } from "../../../modules/mirror/module";

/**
 * La página solo delega en el `Screen` del módulo. Toda la lógica vive bajo
 * `src/modules/mirror/`, así que agregar un módulo es crear su carpeta y una
 * página de tres líneas como esta.
 */
export default function MirrorPage() {
  return <mirror.Screen />;
}
