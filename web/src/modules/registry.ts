import type { ModuleDef } from "./types";
import { mirror } from "./mirror/module";
import { operaciones } from "./operaciones/module";

/**
 * El registro de módulos del frontend.
 *
 * Es el equivalente de la lista `mods` en `cmd/api/main.go`: agregar un módulo es
 * crear su carpeta bajo `src/modules/` y añadir una línea aquí.
 *
 * Ojo: esta lista dice qué módulos SABE dibujar el frontend, no cuáles puede ver
 * el usuario. Eso lo decide el backend en `/api/me`, y el menú se arma cruzando
 * ambas cosas (ver `visibleModules`).
 */
export const modules: ModuleDef[] = [
  mirror,
  operaciones,
];

/**
 * Los módulos que este usuario ve: los que el frontend sabe dibujar Y el backend
 * autoriza.
 *
 * La intersección se hace en este sentido a propósito. Si el backend anuncia un
 * módulo que este frontend todavía no conoce (un despliegue a medias), se ignora
 * en vez de romper el menú.
 */
export function visibleModules(allowed: string[]): ModuleDef[] {
  const set = new Set(allowed);
  return modules.filter((m) => set.has(m.name));
}

export function moduleByName(name: string): ModuleDef | undefined {
  return modules.find((m) => m.name === name);
}
