import { expect, it } from "vitest";

import { displayName, formatPhone } from "./phone";

it("los celulares argentinos se parten como se dictan", () => {
  expect(formatPhone("+5491126126437")).toBe("+54 9 11 2612-6437");
  expect(formatPhone("5493514123456")).toBe("+54 9 351 412-3456");
  expect(formatPhone("+5492966123456")).toBe("+54 9 2966 12-3456");
  expect(formatPhone("+541145678901")).toBe("+54 11 4567-8901");
});

it("los demás números quedan con + y los dígitos tal cual", () => {
  expect(formatPhone("+34612345678")).toBe("+34612345678");
  expect(formatPhone("5491")).toBe("+5491");
});

it("un nombre solo se formatea si es un número", () => {
  expect(displayName("+5491135910619")).toBe("+54 9 11 3591-0619");
  expect(displayName("Beto")).toBe("Beto");
});
