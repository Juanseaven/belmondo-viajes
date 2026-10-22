import type { ComponentType } from "react";

/**
 * Otra entrada del menú del mismo módulo, con su propia pantalla. Aparece si y
 * solo si el módulo está autorizado: no es un módulo aparte para el backend.
 */
export interface ModuleEntry {
  label: string;
  path: string;
  Icon: ComponentType<{ "aria-hidden"?: boolean }>;
}

/**
 * Contrato de un módulo en el frontend. Es el espejo de `modules.Module` del
 * backend: cada módulo se describe a sí mismo y el shell no sabe nada de él más
 * allá de esto.
 */
export interface ModuleDef {
  /**
   * Debe coincidir EXACTAMENTE con el `Name()` del módulo en Go: es la clave con
   * la que se cruza contra la lista que devuelve `/api/me` y la que filtra los
   * eventos del websocket.
   */
  name: string;

  /** Etiqueta en el menú lateral. */
  label: string;

  /** Emoji o carácter que acompaña la etiqueta. */
  icon: string;

  /** Ruta bajo el panel: `/mirror`. */
  path: string;

  /** Descripción corta, para la portada del panel. */
  description: string;

  /** Componente de la pantalla principal del módulo. */
  Screen: ComponentType;

  /** Si el módulo pide atención: el punto verde sobre su ícono en la barra. */
  useAttention?: () => boolean;

  /** Más entradas del menú para el mismo módulo, después de la principal. */
  entries?: ModuleEntry[];
}
