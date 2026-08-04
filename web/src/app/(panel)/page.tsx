"use client";

import Link from "next/link";
import { useSession } from "../../kernel/auth/session";
import { visibleModules } from "../../modules/registry";

/** Portada del panel: las tarjetas de los módulos disponibles. */
export default function HomePage() {
  const { user, modules } = useSession();
  const items = visibleModules(modules);

  return (
    <section>
      <header className="page-header">
        <h1>Hola, {user?.fullName.split(" ")[0]}</h1>
        <p>Estas son las herramientas de la agencia.</p>
      </header>

      <div className="cards">
        {items.map((m) => (
          <Link key={m.name} href={m.path} className="card">
            <span className="card-icon" aria-hidden>
              {m.icon}
            </span>
            <strong>{m.label}</strong>
            <p>{m.description}</p>
          </Link>
        ))}
      </div>
    </section>
  );
}
