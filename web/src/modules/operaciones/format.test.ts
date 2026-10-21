import { expect, it } from "vitest";

import { dayMonth, departs, inWords, longDay, parseAmount, shortDay, whenDue } from "./format";

// 2026-10-20 es martes.
const today = "2026-10-20";

it("un plazo se dice relativo a hoy, con el día de la semana si es esta semana", () => {
  expect(whenDue("2026-10-18", today)).toBe("venció el 18 oct");
  expect(whenDue("2026-10-20T00:00:00Z", today)).toBe("vence hoy");
  expect(whenDue("2026-10-21", today)).toBe("vence mañana");
  expect(whenDue("2026-10-22", today)).toBe("vence el jueves");
  expect(whenDue("2026-10-26", today)).toBe("vence el lunes");
  expect(whenDue("2026-10-27", today)).toBe("vence el 27 oct");
});

it("la salida se dice igual que un plazo", () => {
  expect(departs("2026-10-20", today)).toBe("sale hoy");
  expect(departs("2026-10-21", today)).toBe("sale mañana");
  expect(departs("2026-10-24", today)).toBe("sale el sábado");
  expect(departs("2026-12-10T00:00:00Z", today)).toBe("sale el 10 dic");
  expect(departs("2026-10-03", today)).toBe("salió el 3 oct");
});

it("un plazo en palabras usa el día de la semana solo si cae esta semana", () => {
  const wednesday = "2026-10-21";
  expect(inWords("2026-10-20", wednesday)).toBe("Ayer");
  expect(inWords("2026-10-18", wednesday)).toBe("Hace 3 días");
  expect(inWords("2026-10-21", wednesday)).toBe("Hoy");
  expect(inWords("2026-10-22", wednesday)).toBe("Mañana");
  expect(inWords("2026-10-23", wednesday)).toBe("Viernes");
  expect(inWords("2026-10-25", wednesday)).toBe("Domingo");
  expect(inWords("2026-10-27", wednesday)).toBe("En 6 días");
});

it("una fecha suelta lleva el día de la semana", () => {
  expect(longDay("2026-10-29T00:00:00Z")).toBe("jue 29/10/2026");
  expect(shortDay("2026-11-07")).toBe("sáb 07/11");
  expect(dayMonth("2026-10-20")).toBe("20/10");
});

it("los montos se leen como se escriben en Argentina", () => {
  expect(parseAmount("1450,50")).toBe(1450.5);
  expect(parseAmount("4.641.600")).toBe(4641600);
  expect(parseAmount("")).toBeUndefined();
  expect(parseAmount("tres mil")).toBeNaN();
});
