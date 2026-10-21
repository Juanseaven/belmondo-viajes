import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { NewTripPanel } from "./NewTripPanel";

afterEach(cleanup);

const countries = [
  { code: "AR", name: "Argentina" }, { code: "UY", name: "Uruguay" }, { code: "MX", name: "México" },
  { code: "PA", name: "Panamá" },
];

it("pide de dónde sale y a dónde va, sin texto libre de destino", () => {
  render(<NewTripPanel contactName="Maso" countries={countries} onCancel={vi.fn()} onCreate={vi.fn()} />);

  expect((screen.getByLabelText("Sale de") as HTMLSelectElement).value).toBe("AR");
  expect(screen.getByLabelText("Destino").tagName).toBe("SELECT");
  expect(screen.getByText("Maso")).toBeTruthy();
});

it("crea el viaje con origen, países y fechas en un solo guardado", async () => {
  const onCreate = vi.fn().mockResolvedValue(undefined);
  render(<NewTripPanel contactName="Maso" countries={countries} onCancel={vi.fn()} onCreate={onCreate} />);

  fireEvent.change(screen.getByLabelText("Destino"), { target: { value: "MX" } });
  fireEvent.click(screen.getByRole("button", { name: "Agregar escala" }));
  fireEvent.change(screen.getByLabelText("Escala"), { target: { value: "PA" } });
  fireEvent.change(screen.getByLabelText("Salida"), { target: { value: "2026-12-10" } });
  fireEvent.change(screen.getByLabelText("Regreso"), { target: { value: "2026-12-20" } });
  expect(screen.getByText("Cambiaste el recorrido y las fechas")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Crear viaje" }));

  await waitFor(() => expect(onCreate).toHaveBeenCalledWith({
    originCountry: "AR",
    itinerary: [{ country: "MX", role: "destino" }, { country: "PA", role: "escala" }],
    departAt: "2026-12-10", returnAt: "2026-12-20",
  }));
});

it("las fechas solas también crean el viaje, saliendo de Argentina", async () => {
  const onCreate = vi.fn().mockResolvedValue(undefined);
  render(<NewTripPanel contactName="Maso" countries={countries} onCancel={vi.fn()} onCreate={onCreate} />);

  fireEvent.change(screen.getByLabelText("Salida"), { target: { value: "2026-12-10" } });
  fireEvent.click(screen.getByRole("button", { name: "Crear viaje" }));

  await waitFor(() => expect(onCreate).toHaveBeenCalledWith({ originCountry: "AR", departAt: "2026-12-10" }));
});

it("volver sin cargar nada no crea ni pregunta", () => {
  const onCancel = vi.fn();
  const onCreate = vi.fn();
  render(<NewTripPanel contactName="Maso" countries={countries} onCancel={onCancel} onCreate={onCreate} />);

  fireEvent.click(screen.getByRole("button", { name: "Volver" }));

  expect(onCancel).toHaveBeenCalled();
  expect(onCreate).not.toHaveBeenCalled();
});

it("si el servidor no lo crea, lo dice", async () => {
  const onCreate = vi.fn().mockRejectedValue(new Error("el contacto ya no existe"));
  render(<NewTripPanel contactName="Maso" countries={countries} onCancel={vi.fn()} onCreate={onCreate} />);

  fireEvent.click(screen.getByRole("button", { name: "Crear viaje" }));

  expect((await screen.findByRole("alert")).textContent).toContain("el contacto ya no existe");
});
