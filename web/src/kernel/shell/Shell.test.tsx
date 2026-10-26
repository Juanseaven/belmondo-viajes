import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { TooltipProvider } from "@/components/ui/tooltip";
import { Shell } from "./Shell";

const navigation = vi.hoisted(() => ({ query: "", path: "/mirror" }));

vi.mock("next/navigation", () => ({
  usePathname: () => navigation.path,
  useRouter: () => ({ replace: vi.fn() }),
  useSearchParams: () => new URLSearchParams(navigation.query),
}));

vi.mock("../auth/session", () => ({
  useSession: () => ({
    user: {
      id: "user-1",
      email: "admin@example.com",
      fullName: "Administrador",
      role: "admin",
      isActive: true,
    },
    modules: ["mirror"],
    loading: false,
    logout: vi.fn(),
  }),
}));

afterEach(() => {
  cleanup();
  navigation.query = "";
  navigation.path = "/mirror";
});

describe("Shell sidebar", () => {
  it("uses a compact rail and reveals the CRM labels on hover", () => {
    render(
      <TooltipProvider>
        <Shell><div>Contenido</div></Shell>
      </TooltipProvider>,
    );

    const sidebar = screen.getByRole("complementary");
    expect(sidebar.style.width).toBe("56px");
    expect(screen.queryByText("Bandeja de entrada")).toBeNull();
    expect(screen.getByRole("link", { name: "Bandeja de entrada" })).toBeTruthy();

    fireEvent.mouseEnter(sidebar);

    expect(sidebar.style.width).toBe("208px");
    expect(screen.getByText("Bandeja de entrada")).toBeTruthy();
    expect(screen.getByText("Administrador", { selector: "strong" })).toBeTruthy();

    fireEvent.mouseLeave(sidebar);
    expect(sidebar.style.width).toBe("56px");
  });

  it("closes when another screen opens, even if the clicked link keeps the focus", () => {
    const view = render(
      <TooltipProvider>
        <Shell><div>Contenido</div></Shell>
      </TooltipProvider>,
    );
    const sidebar = screen.getByRole("complementary");
    fireEvent.mouseEnter(sidebar);
    screen.getByRole("link", { name: "Bandeja de entrada" }).focus();
    expect(sidebar.style.width).toBe("208px");

    navigation.path = "/viajes";
    view.rerender(
      <TooltipProvider>
        <Shell><div>Otra pantalla</div></Shell>
      </TooltipProvider>,
    );

    expect(screen.getByRole("complementary").style.width).toBe("56px");
  });

  it("carries no branding of its own", () => {
    render(
      <TooltipProvider>
        <Shell><div>Contenido</div></Shell>
      </TooltipProvider>,
    );

    const sidebar = screen.getByRole("complementary");
    fireEvent.mouseEnter(sidebar);

    expect(sidebar.querySelector(".brand-mark")).toBeNull();
    expect(screen.queryByRole("link", { name: "Ir al inicio" })).toBeNull();
  });

  it("groups the inbox under Principal without future placeholders", () => {
    navigation.query = "";

    render(
      <TooltipProvider>
        <Shell><div>Contenido</div></Shell>
      </TooltipProvider>,
    );

    const sidebar = screen.getByRole("complementary");
    fireEvent.mouseEnter(sidebar);

    const section = screen.getByText("PRINCIPAL");
    const inbox = screen.getByRole("link", { name: "Bandeja de entrada" });
    expect(section.nextElementSibling).toBe(inbox);
    expect(screen.queryByText("Más módulos en camino")).toBeNull();
    expect(screen.getAllByRole("link")).toHaveLength(1);
  });
});
