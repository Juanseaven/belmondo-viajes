"use client";

import { useEffect, useRef } from "react";
import { token } from "./client";

const WS_URL = process.env.NEXT_PUBLIC_WS_URL ?? "ws://localhost:8080";

export interface ServerEvent<T = unknown> {
  /** Qué módulo lo emite. Ver `ws.Event` en el backend. */
  module: string;
  type: string;
  instanceId?: string;
  payload: T;
  at: string;
}

/**
 * Conexión de tiempo real del panel. Una sola para todos los módulos: el
 * servidor filtra por módulo y por línea, así que una pestaña no recibe tráfico
 * que no está mirando.
 *
 * El token viaja por query param porque el navegador no permite mandar cabeceras
 * en el handshake de un WebSocket.
 */
export function useSocket(
  moduleName: string,
  onEvent: (ev: ServerEvent) => void,
  instances: string[] = [],
) {
  // El handler se guarda en una ref para que cambiarlo no reconecte el socket:
  // si dependiera de él, cada render nuevo tiraría la conexión.
  const handler = useRef(onEvent);
  handler.current = onEvent;

  const instanceKey = instances.join(",");

  useEffect(() => {
    const jwt = token.get();
    if (!jwt) return;

    const socket = new WebSocket(`${WS_URL}/api/ws?token=${encodeURIComponent(jwt)}`);

    socket.onopen = () => {
      socket.send(
        JSON.stringify({
          action: "subscribe",
          modules: [moduleName],
          instances: instanceKey ? instanceKey.split(",") : [],
        }),
      );
    };

    socket.onmessage = (msg) => {
      try {
        handler.current(JSON.parse(msg.data) as ServerEvent);
      } catch {
        // Un evento ilegible no puede tumbar la conexión.
      }
    };

    return () => socket.close();
  }, [moduleName, instanceKey]);
}
