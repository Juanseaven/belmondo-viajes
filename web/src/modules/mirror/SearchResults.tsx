"use client";

import { useEffect, useState } from "react";
import { ApiError } from "../../kernel/api/client";
import { displayName } from "@/lib/phone";
import { mirrorApi, type SearchHit } from "./api";
import { listTime } from "./ConversationList";

/**
 * Resultados de buscar dentro de los mensajes.
 *
 * Es la parte que responde "¿qué le prometimos a este cliente?". Abrir un
 * resultado no lleva al final del chat sino AL mensaje: por eso pasa el
 * `messageId` hacia arriba, que termina en el parámetro `around` del backend.
 */
export function SearchResults({
  query,
  instance,
  onOpen,
  onClear,
}: {
  query: string;
  instance: string;
  onOpen: (hit: SearchHit) => void;
  onClear: () => void;
}) {
  const [hits, setHits] = useState<SearchHit[]>([]);
  const [cursor, setCursor] = useState<string | undefined>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let vigente = true;
    setLoading(true);
    setError(null);

    mirrorApi
      .search({ q: query, instance })
      .then((page) => {
        if (!vigente) return;
        setHits(page.items);
        setCursor(page.nextCursor);
      })
      .catch((err: unknown) => {
        if (!vigente) return;
        // El backend distingue "muy corto" de "muy amplio" y ambos son
        // accionables por el usuario: se muestran tal cual, no como error genérico.
        setError(err instanceof ApiError ? err.message : "No se pudo buscar");
      })
      .finally(() => {
        if (vigente) setLoading(false);
      });

    return () => {
      vigente = false;
    };
  }, [query, instance]);

  const loadMore = () => {
    if (!cursor) return;
    void mirrorApi
      .search({ q: query, instance, cursor })
      .then((page) => {
        setHits((prev) => [...prev, ...page.items]);
        setCursor(page.nextCursor);
      })
      .catch(() => undefined);
  };

  return (
    <div className="search-results">
      <header>
        <span>
          Resultados de <strong>{query}</strong>
        </span>
        <button type="button" onClick={onClear}>
          Limpiar
        </button>
      </header>

      {loading && <p className="search-state">Buscando…</p>}
      {error && <p className="search-state error">{error}</p>}
      {!loading && !error && hits.length === 0 && (
        <p className="search-state">No hay mensajes que digan «{query}».</p>
      )}

      <ul>
        {hits.map((hit) => (
          <li key={hit.messageId}>
            <button type="button" className="hit" onClick={() => onOpen(hit)}>
              <span className="hit-top">
                <strong>{displayName(hit.contactName)}</strong>
                <time dateTime={hit.timestamp}>{listTime(hit.timestamp)}</time>
              </span>
              <span className="snippet">{hit.snippet}</span>
              <small>Línea {hit.instanceLabel}</small>
            </button>
          </li>
        ))}
      </ul>

      {cursor && (
        <button type="button" className="more" onClick={loadMore}>
          Ver más resultados
        </button>
      )}
    </div>
  );
}
