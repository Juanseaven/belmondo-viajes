import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { OperationPanel } from "./OperationPanel";
import { operationFor } from "./demoOperations";
import { DEMO_CONVERSATIONS, demoProfile } from "./demoData";

afterEach(cleanup);

const conversation = DEMO_CONVERSATIONS[0];
const profile = demoProfile(conversation);
const operation = operationFor("demo-1")!;

const panel = (overrides: Partial<Parameters<typeof OperationPanel>[0]> = {}) =>
  render(
    <OperationPanel
      conversation={conversation}
      profile={profile}
      operation={operation}
      onRunControl={vi.fn()}
      onConfirmControl={vi.fn()}
      onReopenControl={vi.fn()}
      {...overrides}
    />,
  );

describe("OperationPanel", () => {
  it("identifica la operación sin repetir su estado", () => {
    panel();

    expect(screen.getByText("Operación #10482")).toBeTruthy();
    expect(screen.queryByText("Cotizada")).toBeNull();
    expect(screen.getByText("Iguazú")).toBeTruthy();
  });

  it("muestra el control crítico y no un porcentaje de avance", () => {
    panel();

    expect(screen.getByText("Vencimiento de DNI o pasaporte")).toBeTruthy();
    // Una barra de progreso invita a completarla tildando; es justo lo que se evita.
    expect(screen.queryByText(/%/)).toBeNull();
    expect(screen.queryByRole("progressbar")).toBeNull();
    // Se cuenta lo que falta, no lo que ya está.
    expect(screen.getByText("19 sin controlar")).toBeTruthy();
  });

  it("agrupa los renglones por planilla y los deja todos a la vista", () => {
    panel();

    expect(screen.getByText("Carga de reserva")).toBeTruthy();
    expect(screen.getByText("Control de reserva")).toBeTruthy();
    expect(screen.getByText("10 de 14")).toBeTruthy();
    expect(screen.getByText("2 de 17")).toBeTruthy();
    // Los treinta y un renglones de las dos hojas, controlados o no.
    expect(screen.getAllByRole("checkbox")).toHaveLength(31);
  });

  it("deja registrado quién cerró un control y cuándo", () => {
    panel();

    const done = screen.getByText("Tel y mail del pax cargados en la reserva").closest("li");
    expect(done).not.toBeNull();
    expect(within(done!).getByText(/Juan Pérez/)).toBeTruthy();
  });

  it("mantiene el tilde dentro de la casilla", () => {
    panel();

    const checkbox = screen.getByRole("checkbox", { name: "Carga de reserva: Nombres completos" });
    const check = checkbox.querySelector("svg");
    expect(check?.getAttribute("width")).toBe("11");
    expect(check?.getAttribute("height")).toBe("11");
  });

  it("ofrece la acción real en los controles que se cierran haciendo la cosa", () => {
    const onRunControl = vi.fn();
    panel({ onRunControl });

    fireEvent.click(screen.getByRole("button", { name: "Avisar al pasajero" }));

    expect(onRunControl).toHaveBeenCalledTimes(1);
    expect(onRunControl.mock.calls[0][0].id).toBe("control-vto-doc");
  });

  it("deja tildar a mano cualquier renglón sin controlar", () => {
    const onConfirmControl = vi.fn();
    panel({ onConfirmControl });

    const casilla = screen.getByRole("checkbox", { name: "Control de reserva: Régimen de comidas" });
    expect(casilla.getAttribute("aria-checked")).toBe("false");
    fireEvent.click(casilla);

    expect(onConfirmControl).toHaveBeenCalledTimes(1);
    expect(onConfirmControl.mock.calls[0][0].closing).toBe("manual");
  });

  it("permite destildar lo que se firmó a mano", () => {
    const onReopenControl = vi.fn();
    panel({ onReopenControl });

    const casilla = screen.getByRole("checkbox", { name: "Carga de reserva: Nombres completos" });
    expect(casilla.getAttribute("aria-checked")).toBe("true");
    fireEvent.click(casilla);

    expect(onReopenControl).toHaveBeenCalledTimes(1);
  });

  it("no deja destildar lo que ya salió por la conversación", () => {
    const onReopenControl = vi.fn();
    panel({ operation: operationFor("demo-3")!, onReopenControl });

    // El comprobante llegó por WhatsApp: destildarlo sería negar un hecho.
    const casilla = screen.getByRole("checkbox", {
      name: "Carga de reserva: Comprobante de pago y de carga en el portal, si es con TJ",
    });
    expect(casilla).toHaveProperty("disabled", true);
    fireEvent.click(casilla);

    expect(onReopenControl).not.toHaveBeenCalled();
  });

  it("lista a los pasajeros con su documento y marca el que vence antes del viaje", () => {
    panel();
    fireEvent.click(screen.getByRole("tab", { name: "Pasajeros" }));

    expect(screen.getByText("Pérez, Ana")).toBeTruthy();
    expect(screen.getByText("Titular")).toBeTruthy();

    const sofia = screen.getByText("Pérez, Sofía").closest("li");
    expect(within(sofia!).getByText(/vence/).className).toContain("expired");

    const ana = screen.getByText("Pérez, Ana").closest("li");
    expect(within(ana!).getByText(/vence/).className).not.toContain("expired");
  });

  it("muestra los servicios sin el resumen de liquidación", () => {
    panel();
    fireEvent.click(screen.getByRole("tab", { name: "Servicios" }));

    expect(screen.getByText("Hotel Saint George, 3 noches")).toBeTruthy();
    expect(screen.queryByText("Liquidación coincide")).toBeNull();
    expect(screen.queryByText(/Servicios USD 1\.840/)).toBeNull();
  });

  it("desagrega la plata como la pide la planilla de carga", () => {
    panel();
    fireEvent.click(screen.getByRole("tab", { name: "Pagos" }));

    const value = (label: string) =>
      screen.getByText(label).closest(".detail-row")!.querySelector("strong")!.textContent;

    expect(value("Total")).toBe("USD 1.840");
    expect(value("En pesos")).toBe("ARS 2.154.640");
    expect(value("Cambio del día")).toBe("1.171");
    expect(value("Método")).toBe("Transferencia en pesos");
    expect(value("Seña")).toContain("sin confirmar");
    // La seña no está pagada, así que el saldo sigue siendo el total.
    expect(value("Saldo")).toBe("USD 1.840");
    expect(value("No reembolsable")).toBe("USD 500");
  });

  it("conserva la procedencia de cada documento", () => {
    panel();
    fireEvent.click(screen.getByRole("tab", { name: "Documentos" }));

    const dni = screen.getByText("DNI de Ana Pérez").closest("li");
    expect(within(dni!).getByText(/Juan Pérez/)).toBeTruthy();
  });

  it("distingue el documento que llegó por WhatsApp del que subió una persona", () => {
    const cancun = operationFor("demo-3")!;
    panel({ operation: cancun });
    fireEvent.click(screen.getByRole("tab", { name: "Documentos" }));

    const comprobante = screen.getByText("Comprobante de transferencia").closest("li");
    expect(within(comprobante!).getByText(/Llegó por WhatsApp/)).toBeTruthy();
  });

  it("incorpora el mail, que media planilla necesita", () => {
    panel();

    // Contacto arranca plegado: el expediente completo no entra de una vez.
    expect(screen.queryByText("ana.perez@gmail.com")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: /Contacto/ }));

    expect(screen.getByText("ana.perez@gmail.com")).toBeTruthy();
  });

  it("pliega y despliega cada planilla por separado", () => {
    panel();

    const carga = screen.getByRole("button", { name: /Carga de reserva/ });
    expect(carga.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getAllByRole("checkbox")).toHaveLength(31);

    fireEvent.click(carga);

    expect(carga.getAttribute("aria-expanded")).toBe("false");
    // Se pliegan los catorce de carga y quedan los diecisiete de control.
    expect(screen.getAllByRole("checkbox")).toHaveLength(17);
  });

  it("mantiene el aviso crítico visible aunque se plieguen las planillas", () => {
    panel();

    fireEvent.click(screen.getByRole("button", { name: /Control de reserva/ }));

    expect(screen.getByText("Hay un control crítico sin resolver.")).toBeTruthy();
  });

  it("no muestra etiquetas ni ciudad", () => {
    panel();

    expect(screen.queryByText(/Etiquetas/i)).toBeNull();
    expect(screen.queryByText(/Vicente López/)).toBeNull();
    expect(screen.queryByText("Recurrente")).toBeNull();
  });

  it("dice cuando la conversación todavía no es un viaje", () => {
    panel({ operation: undefined });

    expect(screen.getByText("Sin operación")).toBeTruthy();
    expect(screen.queryByRole("tab", { name: "Servicios" })).toBeNull();
  });
});
