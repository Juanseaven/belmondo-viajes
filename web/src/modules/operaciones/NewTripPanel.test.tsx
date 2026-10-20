import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { NewTripPanel } from "./NewTripPanel";

afterEach(cleanup);

it("muestra la ficha editable sin crear una operación al abrir el chat", () => {
  const onCreate = vi.fn();

  render(<NewTripPanel contactName="Maso" onCreate={onCreate} />);

  expect(screen.getByText("Operación nueva")).toBeTruthy();
  expect(screen.getByRole("tab", { name: "Resumen" })).toBeTruthy();
  expect(screen.getByRole("tab", { name: "Pasajeros" })).toBeTruthy();
  expect(screen.getByRole("tab", { name: "Pagos" })).toBeTruthy();
  expect(screen.getByRole("tab", { name: "Controles" })).toBeTruthy();
  expect(onCreate).not.toHaveBeenCalled();
});

const countries = [
  { code: "AR", name: "Argentina" }, { code: "UY", name: "Uruguay" }, { code: "MX", name: "México" },
  { code: "PA", name: "Panamá" },
];

it("pide de dónde sale y a dónde va, sin texto libre de destino", () => {
  render(<NewTripPanel contactName="Maso" countries={countries} onCreate={vi.fn()} />);

  expect((screen.getByLabelText("Sale de") as HTMLSelectElement).value).toBe("AR");
  expect(screen.getByLabelText("Destino").tagName).toBe("SELECT");
  expect(screen.queryByPlaceholderText("Sin completar")).toBeNull();
  expect((screen.getByRole("button", { name: "Guardar viaje" }) as HTMLButtonElement).disabled).toBe(true);
});

it("crea la operación con origen, países y fechas en un solo guardado", async () => {
  const onCreate = vi.fn().mockResolvedValue(undefined);
  render(<NewTripPanel contactName="Maso" countries={countries} onCreate={onCreate} />);

  fireEvent.change(screen.getByLabelText("Sale de"), { target: { value: "UY" } });
  fireEvent.change(screen.getByLabelText("Destino"), { target: { value: "MX" } });
  fireEvent.click(screen.getByRole("button", { name: "Agregar escala" }));
  fireEvent.change(screen.getByLabelText("Escala"), { target: { value: "PA" } });
  fireEvent.change(screen.getByLabelText("Salida"), { target: { value: "2026-12-10" } });
  fireEvent.click(screen.getByRole("button", { name: "Guardar viaje" }));

  await waitFor(() => expect(onCreate).toHaveBeenCalledWith({
    originCountry: "UY", departAt: "2026-12-10",
    itinerary: [{ country: "MX", role: "destino" }, { country: "PA", role: "escala" }],
  }));
});

it("las fechas solas también crean la operación, saliendo de Argentina", async () => {
  const onCreate = vi.fn().mockResolvedValue(undefined);
  render(<NewTripPanel contactName="Maso" countries={countries} onCreate={onCreate} />);

  fireEvent.change(screen.getByLabelText("Salida"), { target: { value: "2026-12-10" } });
  fireEvent.click(screen.getByRole("button", { name: "Guardar viaje" }));

  await waitFor(() => expect(onCreate).toHaveBeenCalledWith({ originCountry: "AR", departAt: "2026-12-10" }));
});

it("permite que un pasajero sea el primer dato de la operación", async () => {
  const onAddPassenger = vi.fn().mockResolvedValue(undefined);
  const props = {
    contactName: "Maso",
    onCreate: vi.fn(),
    onAddPassenger,
  } as Parameters<typeof NewTripPanel>[0];
  render(<NewTripPanel {...props} />);

  fireEvent.click(screen.getByRole("tab", { name: "Pasajeros" }));
  fireEvent.change(screen.getByLabelText("Nombre"), { target: { value: "Ana" } });
  fireEvent.change(screen.getByLabelText("Apellido"), { target: { value: "Pérez" } });
  fireEvent.click(screen.getByRole("button", { name: "Guardar pasajero" }));

  await waitFor(() => expect(onAddPassenger).toHaveBeenCalledWith({ firstName: "Ana", lastName: "Pérez" }));
});

it("abriendo un viaje nuevo sobre otro existente, se puede volver sin crear nada", () => {
  const onCreate = vi.fn();
  const onCancel = vi.fn();
  render(<NewTripPanel contactName="Maso" onCreate={onCreate} onCancel={onCancel} />);

  fireEvent.click(screen.getByRole("button", { name: "Volver al viaje" }));

  expect(onCancel).toHaveBeenCalled();
  expect(onCreate).not.toHaveBeenCalled();
});
