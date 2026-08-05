"use client";

import type { Line } from "./api";

/**
 * Filtro por línea, con el contador de sin-revisar de cada una.
 *
 * El contador es lo que hace útil el filtro: sin él habría que ir entrando línea
 * por línea para saber dónde hubo movimiento, que es justo lo que la bandeja
 * unificada evita.
 */
export function LineFilter({
  lines,
  selected,
  onSelect,
}: {
  lines: Line[];
  selected: string;
  onSelect: (instanceId: string) => void;
}) {
  const total = lines.reduce((sum, l) => sum + l.unread, 0);

  return (
    <nav className="line-filter" aria-label="Filtrar por línea">
      <button
        type="button"
        className={selected === "" ? "chip active" : "chip"}
        onClick={() => onSelect("")}
      >
        Todas
        {total > 0 && <span className="badge">{total}</span>}
      </button>

      {lines.map((line) => (
        <button
          key={line.id}
          type="button"
          className={selected === line.id ? "chip active" : "chip"}
          onClick={() => onSelect(line.id)}
          title={line.phoneNumber ?? line.name}
        >
          {/* El punto de estado es lo primero que hay que ver: mientras una línea
              está caída, de ese número no entra ni un mensaje. */}
          <span className={`dot ${line.status}`} aria-hidden />
          {line.displayName || line.label}
          {line.unread > 0 && <span className="badge">{line.unread}</span>}
        </button>
      ))}
    </nav>
  );
}
