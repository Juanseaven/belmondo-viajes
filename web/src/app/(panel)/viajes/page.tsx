import { operaciones } from "../../../modules/operaciones/module";

/** La página solo delega en el `Screen` del módulo, como la del espejo. */
export default function ViajesPage() {
  return <operaciones.Screen />;
}
