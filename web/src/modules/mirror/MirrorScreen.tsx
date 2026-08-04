"use client";

/**
 * Pantalla del módulo espejo.
 *
 * Placeholder deliberado: la API del módulo (bandeja, mensajes, envío, alta de
 * línea con QR) es el alcance del spec siguiente. Lo que esta pantalla demuestra
 * hoy es que el andamiaje funciona de punta a punta — el módulo se registra, el
 * shell lo dibuja y la conexión de tiempo real ya filtra por módulo.
 */

import { useState } from "react";
import { useSocket, type ServerEvent } from "../../kernel/api/useSocket";
import { mirror } from "./module";

export function MirrorScreen() {
  const [events, setEvents] = useState<ServerEvent[]>([]);

  useSocket(mirror.name, (ev) => {
    // Solo los últimos 20: esto es una ventana de diagnóstico, no la bandeja.
    setEvents((prev) => [ev, ...prev].slice(0, 20));
  });

  return (
    <section>
      <header className="page-header">
        <h1>WhatsApp</h1>
        <p>{mirror.description}</p>
      </header>

      <div className="notice">
        <strong>La bandeja todavía no está construida.</strong>
        <p>
          La ingesta sí funciona: los mensajes que llegan a cualquier línea se
          persisten y se emiten en tiempo real. Abajo se ven los eventos crudos
          según entran. Listar conversaciones, abrir un hilo, enviar y dar de alta
          una línea con QR es el alcance del próximo spec.
        </p>
      </div>

      <h2>Eventos en vivo</h2>
      {events.length === 0 ? (
        <p className="empty">
          Sin eventos todavía. Escribí a una línea conectada y aparecerá acá.
        </p>
      ) : (
        <ul className="events">
          {events.map((ev, i) => (
            <li key={`${ev.at}-${i}`}>
              <code>{ev.type}</code>
              <time>{new Date(ev.at).toLocaleTimeString()}</time>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
