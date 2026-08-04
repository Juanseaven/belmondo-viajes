"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, type ReactNode } from "react";
import { useSession } from "../auth/session";
import { visibleModules } from "../../modules/registry";

/**
 * Marco del panel: barra lateral con los módulos y guarda de sesión.
 *
 * El menú NO sale del registro del frontend a secas, sino de cruzarlo con lo que
 * `/api/me` autoriza. El backend es la autoridad sobre qué existe; el frontend
 * solo sabe dibujarlo.
 */
export function Shell({ children }: { children: ReactNode }) {
  const { user, modules, loading, logout } = useSession();
  const router = useRouter();
  const pathname = usePathname();

  useEffect(() => {
    if (!loading && !user) router.replace("/login");
  }, [loading, user, router]);

  if (loading) return <div className="centered">Cargando…</div>;
  if (!user) return null;

  const items = visibleModules(modules);

  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="brand">viajes_crm</div>

        <nav>
          {items.map((m) => (
            <Link
              key={m.name}
              href={m.path}
              className={pathname.startsWith(m.path) ? "nav-item active" : "nav-item"}
            >
              <span aria-hidden>{m.icon}</span>
              {m.label}
            </Link>
          ))}
          {items.length === 0 && (
            <p className="empty">No tenés módulos habilitados.</p>
          )}
        </nav>

        <footer className="user">
          <div>
            <strong>{user.fullName}</strong>
            <small>{user.role === "admin" ? "Administrador" : "Equipo"}</small>
          </div>
          <button type="button" onClick={logout}>
            Salir
          </button>
        </footer>
      </aside>

      <main className="content">{children}</main>
    </div>
  );
}
