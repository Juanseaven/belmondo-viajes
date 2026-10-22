import { UsersRound } from "lucide-react";
import type { ModuleDef } from "../types";
import { ViajesScreen } from "./ViajesScreen";
import "./operaciones.css";
import "./clientes.css";

/**
 * Descriptor del módulo operaciones. Su pantalla es Viajes, y suma Clientes
 * como segunda entrada del menú; la ficha del viaje, en cambio, la dibuja la
 * pantalla del espejo al lado de cada conversación.
 *
 * `name` debe coincidir con `domain.ModuleName` del backend.
 */
export const operaciones: ModuleDef = {
  name: "operaciones",
  label: "Viajes",
  icon: "✈",
  path: "/viajes",
  description: "Lo que hay que hacer con cada viaje hoy, y la consulta de todos los viajes de la agencia.",
  Screen: ViajesScreen,
  entries: [{ label: "Clientes", path: "/clientes", Icon: UsersRound }],
};
