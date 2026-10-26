const DEVELOPMENT_API_URL = "http://localhost:8080";
const DEVELOPMENT_WS_URL = "ws://localhost:8080";

export function resolveApiBaseUrl(configured = process.env.NEXT_PUBLIC_API_URL): string {
  if (configured === "same-origin") return "";
  return configured ?? DEVELOPMENT_API_URL;
}

export function resolveWebSocketBaseUrl(
  configured: string | undefined,
  location: Pick<Location, "protocol" | "host">,
): string {
  if (configured !== "same-origin") return configured ?? DEVELOPMENT_WS_URL;

  const protocol = location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${location.host}`;
}
