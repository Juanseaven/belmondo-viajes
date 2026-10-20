import { redirect } from "next/navigation";
import { homePath } from "../../modules/registry";

/**
 * El panel no tiene portada: arranca en la bandeja de WhatsApp, que es donde la
 * agencia trabaja todo el día. La ruta existe para que un favorito o el login
 * sigan llevando a algún lado.
 */
export default function HomePage() {
  redirect(homePath);
}
