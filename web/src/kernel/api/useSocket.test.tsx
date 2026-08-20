import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { useSocket } from "./useSocket";

class FakeWebSocket {
  static instances: FakeWebSocket[] = [];

  readonly send = vi.fn();
  readonly close = vi.fn();
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;

  constructor(readonly url: string) {
    FakeWebSocket.instances.push(this);
  }
}

beforeEach(() => {
  vi.useFakeTimers();
  FakeWebSocket.instances = [];
  window.localStorage.setItem("viajes_crm_token", "jwt-real");
  vi.stubGlobal("WebSocket", FakeWebSocket);
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  window.localStorage.clear();
});

it("reconecta y repone la suscripcion cuando el websocket se corta", () => {
  const { unmount } = renderHook(() => useSocket("mirror", vi.fn(), ["linea-real"]));
  expect(FakeWebSocket.instances).toHaveLength(1);

  act(() => {
    FakeWebSocket.instances[0].onclose?.(new CloseEvent("close"));
    vi.advanceTimersByTime(1_000);
  });

  expect(FakeWebSocket.instances).toHaveLength(2);
  act(() => FakeWebSocket.instances[1].onopen?.(new Event("open")));
  expect(FakeWebSocket.instances[1].send).toHaveBeenCalledWith(JSON.stringify({
    action: "subscribe",
    modules: ["mirror"],
    instances: ["linea-real"],
  }));

  unmount();
});
