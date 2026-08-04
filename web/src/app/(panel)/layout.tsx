import type { ReactNode } from "react";
import { Shell } from "../../kernel/shell/Shell";

/**
 * Todo lo que cuelgue de `(panel)` queda dentro del shell y detrás de la guarda
 * de sesión. Una página de módulo nueva no tiene que acordarse de nada.
 */
export default function PanelLayout({ children }: { children: ReactNode }) {
  return <Shell>{children}</Shell>;
}
