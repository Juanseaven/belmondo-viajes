/**
 * Cliente HTTP del panel. Único punto por el que sale una petición al backend:
 * el token y la forma de los errores se manejan aquí y en ningún otro sitio.
 */

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

/** Forma de error del backend: `{ error: { code, message } }` (ver kernel/httpx). */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }

  /** La sesión caducó o el usuario fue dado de baja: hay que volver al login. */
  get isAuthError(): boolean {
    return this.status === 401;
  }
}

const TOKEN_KEY = "viajes_crm_token";

export const token = {
  get: (): string | null =>
    typeof window === "undefined" ? null : window.localStorage.getItem(TOKEN_KEY),
  set: (value: string) => window.localStorage.setItem(TOKEN_KEY, value),
  clear: () => window.localStorage.removeItem(TOKEN_KEY),
};

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const jwt = token.get();
  const res = await fetch(`${API_URL}${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(jwt ? { Authorization: `Bearer ${jwt}` } : {}),
      ...init.headers,
    },
  });

  if (res.status === 204) return undefined as T;

  const body = await res.json().catch(() => null);

  if (!res.ok) {
    // Si el cuerpo no trae la forma esperada (un 502 de un proxy, por ejemplo),
    // se sintetiza un error con la misma forma para que quien llama no tenga que
    // distinguir los dos casos.
    throw new ApiError(
      res.status,
      body?.error?.code ?? "UNKNOWN",
      body?.error?.message ?? `Error ${res.status}`,
    );
  }
  return body as T;
}

export const apiUrl = API_URL;
