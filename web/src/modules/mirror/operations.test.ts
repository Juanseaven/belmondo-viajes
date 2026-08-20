import { describe, expect, it } from "vitest";

import {
  balanceUsd,
  expiringPassengers,
  liquidationMatches,
  nextControl,
  operationRowState,
  servicesTotal,
  type Control,
  type Operation,
} from "./operations";

const control = (id: string, extra: Partial<Control> = {}): Control => ({
  id,
  stage: "control",
  title: id,
  closing: "manual",
  ...extra,
});

const operation = (extra: Partial<Operation> = {}): Operation => ({
  id: "10482",
  status: "confirmada",
  conversationId: "demo-1",
  destination: "Iguazú",
  departAt: "2026-09-12",
  returnAt: "2026-09-15",
  seller: "Juan Pérez",
  createdAt: "2026-08-11T11:39:00-03:00",
  createdBy: "Juan Pérez",
  passengers: [],
  services: [],
  payment: {
    totalUsd: 1840,
    rate: 1171,
    method: "Transferencia",
    depositUsd: 500,
    depositPaid: false,
  },
  documents: [],
  controls: [],
  ...extra,
});

describe("nextControl", () => {
  it("pone el crítico adelante aunque venza después", () => {
    const op = operation({
      controls: [
        control("comun", { dueAt: "2026-08-12" }),
        control("critico", { critical: true, dueAt: "2026-09-01" }),
      ],
    });

    expect(nextControl(op)?.id).toBe("critico");
  });

  it("entre iguales elige el que vence antes", () => {
    const op = operation({
      controls: [control("tarde", { dueAt: "2026-09-01" }), control("temprano", { dueAt: "2026-08-12" })],
    });

    expect(nextControl(op)?.id).toBe("temprano");
  });

  it("manda al final lo que no vence", () => {
    const op = operation({
      controls: [control("sin-fecha"), control("con-fecha", { dueAt: "2026-09-01" })],
    });

    expect(nextControl(op)?.id).toBe("con-fecha");
  });

  it("ignora los controles ya cerrados", () => {
    const op = operation({
      controls: [
        control("cerrado", { critical: true, done: { at: "2026-08-11T10:00:00-03:00", by: "Juan Pérez" } }),
        control("abierto"),
      ],
    });

    expect(nextControl(op)?.id).toBe("abierto");
  });
});

describe("operationRowState", () => {
  it("muestra el control crítico y lo tiñe de urgente", () => {
    const op = operation({ controls: [control("Vence el DNI de Delfina", { critical: true })] });

    expect(operationRowState(op)).toEqual({ label: "Vence el DNI de Delfina", tone: "late" });
  });

  it("muestra el control común como advertencia", () => {
    const op = operation({ controls: [control("Falta el voucher del hotel")] });

    expect(operationRowState(op)).toEqual({ label: "Falta el voucher del hotel", tone: "warn" });
  });

  it("cae al estado de la operación cuando no queda nada abierto", () => {
    const op = operation({
      status: "en viaje",
      controls: [control("listo", { done: { at: "2026-08-11T10:00:00-03:00", by: "Juan Pérez" } })],
    });

    expect(operationRowState(op)).toEqual({ label: "Viaje en curso" });
  });
});

describe("liquidación", () => {
  it("cierra cuando los servicios suman el total cotizado", () => {
    const op = operation({
      services: [
        { id: "a", kind: "aereo", detail: "AEP–IGR", status: "emitido", amountUsd: 1090 },
        { id: "h", kind: "hotel", detail: "Saint George", status: "reservado", amountUsd: 620 },
        { id: "t", kind: "traslado", detail: "In/out", status: "reservado", amountUsd: 130 },
      ],
    });

    expect(servicesTotal(op)).toBe(1840);
    expect(liquidationMatches(op)).toBe(true);
  });

  it("no cierra cuando falta cargar un servicio", () => {
    const op = operation({
      services: [{ id: "a", kind: "aereo", detail: "AEP–IGR", status: "emitido", amountUsd: 1090 }],
    });

    expect(liquidationMatches(op)).toBe(false);
  });
});

describe("saldo", () => {
  it("descuenta la seña solo cuando está pagada", () => {
    expect(balanceUsd(operation())).toBe(1840);

    const senada = operation();
    senada.payment.depositPaid = true;
    expect(balanceUsd(senada)).toBe(1340);
  });
});

describe("expiringPassengers", () => {
  it("detecta el documento que vence antes de volver", () => {
    const op = operation({
      passengers: [
        {
          id: "p1", firstName: "Marina", lastName: "Ferreyra", kind: "adulto", isHolder: true,
          document: { type: "DNI", number: "30.111.222", expiresAt: "2031-04-02" },
        },
        {
          id: "p3", firstName: "Delfina", lastName: "Ferreyra", kind: "menor", isHolder: false,
          document: { type: "DNI", number: "55.987.101", expiresAt: "2026-09-02" },
        },
      ],
    });

    expect(expiringPassengers(op).map((passenger) => passenger.firstName)).toEqual(["Delfina"]);
  });
});
