import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import type { Conversation } from "./api";
import { ContactDetails } from "./ContactDetails";

const conversation: Conversation = {
  id: "conversation-1",
  instanceId: "line-1",
  instanceLabel: "Juanse",
  contactId: "contact-1",
  contactName: "Carla Mendoza",
  phoneNumber: "+5491134567890",
  isGroup: false,
  unread: 0,
};

afterEach(cleanup);

describe("ContactDetails", () => {
  it("muestra los datos del contacto con el teléfono formateado y la línea", () => {
    render(<ContactDetails conversation={conversation}><p>Este contacto no tiene viajes</p></ContactDetails>);

    expect(screen.getByText("Carla Mendoza")).toBeTruthy();
    expect(screen.getByText("+54 9 11 3456-7890")).toBeTruthy();
    expect(screen.getByText("Línea Juanse")).toBeTruthy();
    expect(screen.getByText("Este contacto no tiene viajes")).toBeTruthy();
  });

  it("si el nombre es el número, no lo repite", () => {
    render(<ContactDetails conversation={{ ...conversation, contactName: "+5491134567890" }} />);

    expect(screen.getAllByText("+54 9 11 3456-7890")).toHaveLength(1);
  });
});
