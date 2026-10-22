import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { mirrorApi } from "@/modules/mirror/api";
import { operacionesApi } from "./api";
import { CustomerFiles, period } from "./CustomerFiles";
import type { CustomerFile } from "./types";

const push = vi.fn();

vi.mock("next/navigation", () => ({ useRouter: () => ({ push, replace: vi.fn() }) }));
vi.mock("@/kernel/api/useSocket", () => ({ useSocket: () => undefined }));
vi.mock("./api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./api")>();
  return { ...actual, operacionesApi: { ...actual.operacionesApi, customerFiles: vi.fn(), setFavorite: vi.fn() } };
});
vi.mock("@/modules/mirror/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/modules/mirror/api")>();
  return { ...actual, mirrorApi: { ...actual.mirrorApi, media: vi.fn() } };
});

const daysAgo = (n: number) => {
  const d = new Date();
  d.setDate(d.getDate() - n);
  return d.toISOString();
};

const base = { conversationId: "conv-1", direction: "inbound" as const, status: "stored" as const, favorite: false, instanceLabel: "Juanse" };
const media: CustomerFile[] = [
  { ...base, assetId: "a-1", messageId: "m-1", type: "image", sentAt: daysAgo(1) },
  { ...base, assetId: "a-2", messageId: "m-2", type: "video", sentAt: daysAgo(2), direction: "outbound", durationSeconds: 42 },
  { ...base, assetId: "a-3", messageId: "m-3", type: "image", sentAt: "2025-03-10T15:00:00Z", favorite: true },
];
const docs: CustomerFile[] = [
  { ...base, assetId: "a-4", messageId: "m-4", type: "document", fileName: "Comprobante.pdf", sizeBytes: 186_000, sentAt: daysAgo(1) },
];

beforeEach(() => {
  vi.mocked(operacionesApi.customerFiles).mockImplementation(async (_id, params) => ({
    items: params.tipo === "documentos" ? docs : media,
    counts: { todos: 4, favoritos: 1, documentos: 1 },
  }));
  vi.mocked(operacionesApi.setFavorite).mockResolvedValue(undefined);
  vi.mocked(mirrorApi.media).mockResolvedValue({ url: "https://r2.test/foto.jpg", mimeType: "image/jpeg", expiresIn: 300 });
});
afterEach(() => { cleanup(); vi.clearAllMocks(); });

it("agrupa por período: esta semana y después el mes con año si no es el actual", () => {
  expect(period("2026-10-20T10:00:00Z", "2026-10-22")).toBe("Esta semana");
  expect(period("2026-10-02T10:00:00Z", "2026-10-22")).toBe("Este mes");
  expect(period("2026-09-02T10:00:00Z", "2026-10-22")).toBe("Septiembre");
  expect(period("2025-03-10T10:00:00Z", "2026-10-22")).toBe("Marzo 2025");
});

it("muestra la grilla por período, los documentos aparte y lo de la agencia marcado", async () => {
  render(<CustomerFiles customerId="cliente-1" customerName="Marta" />);

  expect(await screen.findByText("Esta semana")).toBeTruthy();
  expect(screen.getByText("Marzo 2025")).toBeTruthy();
  expect(screen.getByText("Agencia")).toBeTruthy();
  expect(screen.getByText("0:42")).toBeTruthy();
  expect(screen.getByText("Comprobante.pdf")).toBeTruthy();
  expect(screen.getByText(/182 KB · del cliente/)).toBeTruthy();
  expect(screen.getByRole("button", { name: "Favoritos 1" })).toBeTruthy();
  expect(operacionesApi.customerFiles).toHaveBeenCalledWith("cliente-1", expect.objectContaining({ tipo: "media" }));
  expect(operacionesApi.customerFiles).toHaveBeenCalledWith("cliente-1", expect.objectContaining({ tipo: "documentos" }));
});

it("la estrella se marca sin abrir la foto", async () => {
  render(<CustomerFiles customerId="cliente-1" customerName="Marta" />);
  const stars = await screen.findAllByRole("button", { name: "Marcar como favorito" });
  fireEvent.click(stars[0]);

  await waitFor(() => expect(operacionesApi.setFavorite).toHaveBeenCalledWith(expect.objectContaining({ messageId: "m-1" }), true));
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(screen.getByRole("button", { name: "Favoritos 2" })).toBeTruthy();
});

it("el visor pasa al siguiente y «Ver en el chat» lleva al mensaje", async () => {
  render(<CustomerFiles customerId="cliente-1" customerName="Marta" />);
  fireEvent.click((await screen.findAllByRole("button", { name: /^Foto del/ }))[0]);

  const dialog = await screen.findByRole("dialog");
  expect(within(dialog).getByText("Foto del cliente")).toBeTruthy();
  fireEvent.click(within(dialog).getByRole("button", { name: "Siguiente" }));
  expect(within(screen.getByRole("dialog")).getByText("Video de la agencia")).toBeTruthy();

  fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Ver en el chat" }));
  expect(push).toHaveBeenCalledWith("/mirror?c=conv-1&m=m-2");
});

it("en la página del cliente abre en Favoritos si hay alguno", async () => {
  render(<CustomerFiles customerId="cliente-1" customerName="Marta" columns={6} preview={12} />);
  await waitFor(() => expect(screen.getByRole("button", { name: "Favoritos 1" }).getAttribute("aria-pressed")).toBe("true"));
  expect(operacionesApi.customerFiles).toHaveBeenCalledWith("cliente-1", expect.objectContaining({ favoritos: true }));
});
