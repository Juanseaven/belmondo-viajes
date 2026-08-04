import type { ModuleDef } from "../types";
import { MirrorScreen } from "./MirrorScreen";

/**
 * Descriptor del módulo espejo.
 *
 * `name` debe coincidir exactamente con `domain.ModuleName` del backend: es la
 * clave con la que el shell decide si mostrarlo y con la que el websocket filtra
 * sus eventos.
 */
export const mirror: ModuleDef = {
  name: "mirror",
  label: "WhatsApp",
  icon: "💬",
  path: "/mirror",
  description:
    "Todas las líneas de la agencia en una bandeja, con filtro por número y envío desde el panel.",
  Screen: MirrorScreen,
};
