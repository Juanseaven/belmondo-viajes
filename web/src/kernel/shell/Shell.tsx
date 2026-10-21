"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState, type ComponentType, type FocusEvent, type ReactNode } from "react";
import { LogOut, MessageCircleMore, Plane } from "lucide-react";

import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { visibleModules } from "../../modules/registry";
import type { ModuleDef } from "../../modules/types";
import { useSession } from "../auth/session";

const SIDEBAR_WIDTH = 56;
const SIDEBAR_OPEN_WIDTH = 208;

/** Cada módulo trae su ícono; los que no figuran caen en el de mensajería. */
const MODULE_ICONS: Record<string, ComponentType<{ "aria-hidden"?: boolean }>> = {
  mirror: MessageCircleMore,
  operaciones: Plane,
};

function initials(value: string): string {
  const parts = value.trim().split(/\s+/).filter(Boolean);
  if (!parts.length) return "?";
  return `${parts[0][0]}${parts.at(-1)?.[0] ?? ""}`.toUpperCase();
}

/** Un ítem de la barra. Es un componente aparte porque el módulo puede traer su hook de atención. */
function NavItem({ module, open, active }: { module: ModuleDef; open: boolean; active: boolean }) {
  const attention = module.useAttention?.() ?? false;
  const label = module.name === "mirror" ? "Bandeja de entrada" : module.label;
  const Icon = MODULE_ICONS[module.name] ?? MessageCircleMore;
  return (
    <Link
      href={module.path}
      className={cn("nav-item", active && "active", open && "is-open")}
      aria-label={!open ? label : undefined}
      title={!open ? label : undefined}
    >
      {active && <span className="nav-active-rail" aria-hidden />}
      <span className="nav-icon">
        <Icon aria-hidden />
        {attention && <span className="nav-dot" role="img" aria-label="Hay novedades" />}
      </span>
      {open && <span className="nav-label">{label}</span>}
    </Link>
  );
}

export function Shell({ children }: { children: ReactNode }) {
  const { user, modules, loading, logout } = useSession();
  const router = useRouter();
  const pathname = usePathname();
  const [hovered, setHovered] = useState(false);
  const [focusWithin, setFocusWithin] = useState(false);

  useEffect(() => {
    if (!loading && !user) router.replace("/login");
  }, [loading, user, router]);

  if (loading) return <div className="centered">Cargando…</div>;
  if (!user) return null;

  const items = visibleModules(modules);
  const open = hovered || focusWithin;

  function handleBlur(event: FocusEvent<HTMLElement>) {
    if (!event.currentTarget.contains(event.relatedTarget as Node | null)) {
      setFocusWithin(false);
    }
  }

  return (
    <div className="shell">
      <aside
        className={cn("app-sidebar", open && "is-open")}
        style={{ width: open ? SIDEBAR_OPEN_WIDTH : SIDEBAR_WIDTH }}
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
        onFocusCapture={() => setFocusWithin(true)}
        onBlurCapture={handleBlur}
      >
        <nav aria-label="Navegación principal" className="sidebar-scroll">
          {open && <p className="sidebar-section">PRINCIPAL</p>}
          {items.map((module) => (
            <NavItem key={module.name} module={module} open={open} active={pathname.startsWith(module.path)} />
          ))}
          {items.length === 0 && open && <p className="sidebar-empty">No tenés módulos habilitados.</p>}
        </nav>

        <div className="sidebar-separator" />

        <footer className={cn("user-menu", open && "is-open")}>
          {open && (
            <>
              <Avatar className="user-avatar">
                <AvatarFallback>{initials(user.fullName)}</AvatarFallback>
              </Avatar>
              <div className="user-copy">
                <strong>{user.fullName}</strong>
                <small>{user.role === "admin" ? "Administrador" : "Equipo"}</small>
              </div>
            </>
          )}
          <Tooltip>
            <TooltipTrigger asChild>
              <Button type="button" variant="ghost" size="icon" onClick={logout} aria-label="Cerrar sesión">
                <LogOut aria-hidden />
              </Button>
            </TooltipTrigger>
            <TooltipContent side="right">Cerrar sesión</TooltipContent>
          </Tooltip>
        </footer>
      </aside>

      <main className="content">{children}</main>
    </div>
  );
}
