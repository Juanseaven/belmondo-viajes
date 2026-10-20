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

it("crea la operación al guardar el primer dato del viaje", async () => {
  const onCreate = vi.fn().mockResolvedValue(undefined);
  render(<NewTripPanel contactName="Maso" onCreate={onCreate} />);

  fireEvent.change(screen.getByLabelText("Destino"), { target: { value: "Iguazú" } });
  fireEvent.click(screen.getByRole("button", { name: "Guardar viaje" }));

  await waitFor(() => expect(onCreate).toHaveBeenCalledWith({ destination: "Iguazú" }));
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
