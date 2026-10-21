"use client";

import { useEffect, useState } from "react";
import { mirrorApi } from "./api";

const EVENT = "mirror:unread";
const POLL_MS = 30_000;

/** La bandeja avisa cuántos chats sin leer hay cada vez que recarga las líneas. */
export function announceUnread(total: number) {
  window.dispatchEvent(new CustomEvent<number>(EVENT, { detail: total }));
}

/**
 * Si hay chats sin leer, para el punto verde de la barra lateral. Con la bandeja
 * abierta lo sabe al instante por `announceUnread`; en otra pantalla pregunta
 * cada tanto. No abre otro websocket solo para un punto.
 */
export function useUnreadDot(): boolean {
  const [unread, setUnread] = useState(false);

  useEffect(() => {
    let current = true;
    const check = () => {
      mirrorApi.lines()
        .then((res) => { if (current) setUnread(res.lines.some((line) => line.unread > 0)); })
        .catch(() => undefined);
    };
    const onAnnounce = (event: Event) => setUnread((event as CustomEvent<number>).detail > 0);
    check();
    const timer = setInterval(check, POLL_MS);
    window.addEventListener(EVENT, onAnnounce);
    return () => {
      current = false;
      clearInterval(timer);
      window.removeEventListener(EVENT, onAnnounce);
    };
  }, []);

  return unread;
}
