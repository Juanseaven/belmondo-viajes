import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";

import type { Line } from "./api";
import { LineFilter } from "./LineFilter";

const line: Line = {
  id: "line-1",
  name: "juanse-c21c00",
  label: "juanse",
  status: "open",
  isActive: true,
  unread: 0,
};

afterEach(cleanup);

describe("LineFilter line management", () => {
  it("centers the line menu under its selector", async () => {
    const user = userEvent.setup();
    render(<LineFilter lines={[line]} selected="" onSelect={() => undefined} />);

    await user.click(screen.getByRole("button", { name: /todas las líneas/i }));

    expect(screen.getByRole("menu").getAttribute("data-align")).toBe("center");
  });

  it("selects a line from the current-line menu", async () => {
    const user = userEvent.setup();
    let selected = "";
    const north: Line = {
      ...line,
      id: "linea-1",
      label: "Línea Norte",
      displayName: "Línea Norte",
      phoneNumber: "+54 9 11 3456-7890",
    };

    render(
      <LineFilter
        lines={[north]}
        selected=""
        onSelect={(id) => { selected = id; }}
      />,
    );

    await user.click(screen.getByRole("button", { name: /todas las líneas/i }));
    await user.click(screen.getByRole("menuitemradio", { name: /línea norte/i }));

    expect(selected).toBe("linea-1");
  });

  it("requires the line label before unlinking and reports the selected line", async () => {
    const user = userEvent.setup();
    let disconnected = "";
    render(
      <LineFilter
        lines={[line]}
        selected=""
        onSelect={() => undefined}
        canManage
        onDisconnect={async (selectedLine) => {
          disconnected = selectedLine.id;
        }}
      />,
    );

    await user.click(screen.getByRole("button", { name: /todas las líneas/i }));
    await user.click(screen.getByRole("menuitem", { name: "Desvincular juanse" }));

    const confirm = screen.getByRole("button", { name: "Desvincular" });
    expect(confirm).toHaveProperty("disabled", true);
    await user.type(screen.getByLabelText(/Escribí .*juanse.* para confirmar/), "juanse");
    expect(confirm).toHaveProperty("disabled", false);
    await user.click(confirm);

    await waitFor(() => expect(disconnected).toBe("line-1"));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("does not expose line management to non-admin users", async () => {
    const user = userEvent.setup();
    render(<LineFilter lines={[line]} selected="" onSelect={() => undefined} />);

    await user.click(screen.getByRole("button", { name: /todas las líneas/i }));
    expect(screen.queryByRole("menuitem", { name: /Desvincular/ })).toBeNull();
  });
});
