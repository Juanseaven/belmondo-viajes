import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import type { Conversation, Line } from "./api";
import { ContactDetails } from "./ContactDetails";

const conversation: Conversation = {
  id: "conversation-1",
  instanceId: "line-1",
  instanceLabel: "Línea Norte",
  contactId: "contact-1",
  contactName: "Carla Mendoza",
  phoneNumber: "+54 9 11 3456-7890",
  isGroup: false,
  unread: 0,
};

const line: Line = {
  id: "line-1",
  name: "linea-norte",
  label: "Línea Norte",
  phoneNumber: "+54 9 11 3356-6901",
  status: "open",
  isActive: true,
  unread: 0,
};

afterEach(cleanup);

describe("ContactDetails", () => {
  it("shows only real contact data and the current line", () => {
    render(<ContactDetails conversation={conversation} line={line} mode="panel" />);

    expect(screen.getByText("Carla Mendoza")).toBeTruthy();
    expect(screen.getByText("+54 9 11 3456-7890")).toBeTruthy();
    expect(screen.getByText("Línea Norte")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Contacto" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Atención" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Conversación" })).toBeTruthy();
    expect(screen.getByText("Conectada")).toBeTruthy();
    expect(screen.getByText("Sin mensajes pendientes")).toBeTruthy();
    expect(screen.queryByText(/demostración/i)).toBeNull();
    expect(screen.queryByText(/etiquetas/i)).toBeNull();
  });

  it("surfaces the real pending-reply state", () => {
    render(
      <ContactDetails
        conversation={{ ...conversation, unread: 3, needsReply: true, lastMessageAt: "2026-08-18T15:00:00Z" }}
        line={line}
        mode="panel"
      />,
    );

    expect(screen.getByText("Espera respuesta")).toBeTruthy();
    expect(screen.getByText("3 mensajes sin leer")).toBeTruthy();
    expect(screen.getByText("Última actividad")).toBeTruthy();
  });

  it("renders nothing without an open conversation", () => {
    const { container } = render(<ContactDetails conversation={null} mode="panel" />);

    expect(container.textContent).toBe("");
  });
});
