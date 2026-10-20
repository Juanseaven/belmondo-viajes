import { expect, it, vi } from "vitest";

const redirect = vi.fn();
vi.mock("next/navigation", () => ({ redirect: (path: string) => redirect(path) }));

import HomePage from "./page";

// No hay portada: el panel arranca en la bandeja de WhatsApp, que es donde la
// agencia trabaja todo el día. La ruta / sigue existiendo para los favoritos.
it("la raíz del panel lleva a la bandeja", () => {
  HomePage();
  expect(redirect).toHaveBeenCalledWith("/mirror");
});
