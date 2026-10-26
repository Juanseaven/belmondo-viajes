import { describe, expect, it } from "vitest";

import { resolveApiBaseUrl, resolveWebSocketBaseUrl } from "./urls";

describe("resolveApiBaseUrl", () => {
  it("uses relative requests in same-origin mode", () => {
    expect(resolveApiBaseUrl("same-origin")).toBe("");
  });

  it("preserves an explicit API URL and the development default", () => {
    expect(resolveApiBaseUrl("https://api.example.com")).toBe("https://api.example.com");
    expect(resolveApiBaseUrl()).toBe("http://localhost:8080");
  });
});

describe("resolveWebSocketBaseUrl", () => {
  it("derives a secure WebSocket URL from an HTTPS origin", () => {
    expect(resolveWebSocketBaseUrl("same-origin", {
      protocol: "https:",
      host: "demo.trycloudflare.com",
    })).toBe("wss://demo.trycloudflare.com");
  });

  it("derives a local WebSocket URL from an HTTP origin", () => {
    expect(resolveWebSocketBaseUrl("same-origin", {
      protocol: "http:",
      host: "localhost:8088",
    })).toBe("ws://localhost:8088");
  });

  it("preserves an explicit WebSocket URL and the development default", () => {
    const location = { protocol: "https:", host: "ignored.example.com" };

    expect(resolveWebSocketBaseUrl("wss://socket.example.com", location)).toBe("wss://socket.example.com");
    expect(resolveWebSocketBaseUrl(undefined, location)).toBe("ws://localhost:8080");
  });
});
