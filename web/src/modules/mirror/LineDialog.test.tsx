import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { mirrorApi, type Line } from "./api";
import { LineDialog } from "./LineDialog";

vi.mock("./api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./api")>();
  return {
    ...actual,
    mirrorApi: {
      ...actual.mirrorApi,
      lineQR: vi.fn(),
      lines: vi.fn(),
    },
  };
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

it("uses the generic sales example for a new line", () => {
  render(
    <LineDialog
      qrEvents={null}
      onClose={() => undefined}
      onConnected={() => undefined}
    />,
  );

  expect(screen.getByLabelText("Nombre de la línea").getAttribute("placeholder")).toBe("Ej. Ventas");
});

it("shows the QR for an existing revoked line without creating another line", async () => {
  const line: Line = {
    id: "existing-id",
    name: "juanse-existing",
    label: "Juanse",
    status: "connecting",
    isActive: true,
    unread: 0,
  };
  vi.mocked(mirrorApi.lineQR).mockResolvedValue({
    instance: line.name,
    qr: "data:image/png;base64,ZXhpc3Rpbmc=",
  });

  render(
    <LineDialog
      existingLine={line}
      qrEvents={null}
      onClose={() => undefined}
      onConnected={() => undefined}
    />,
  );

  expect(screen.queryByLabelText("Nombre de la línea")).toBeNull();
  expect(screen.queryByRole("button", { name: "Crear y mostrar QR" })).toBeNull();
  await waitFor(() => expect(mirrorApi.lineQR).toHaveBeenCalledWith(line.id));
  expect(await screen.findByRole("img", { name: "Código QR de vinculación" })).toBeTruthy();
});
