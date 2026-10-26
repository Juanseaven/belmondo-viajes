/**
 * Cliente HTTP del panel. Único punto por el que sale una petición al backend:
 * el token y la forma de los errores se manejan aquí y en ningún otro sitio.
 */
import { resolveApiBaseUrl } from "./urls";

const API_URL = resolveApiBaseUrl();

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

async function authenticatedFetch(
  path: string,
  init: RequestInit,
  defaultJSONContentType: boolean,
): Promise<Response> {
  const jwt = token.get();

  // Con FormData el Content-Type lo tiene que poner el navegador: lleva el
  // boundary del multipart, que nosotros no conocemos. Fijarlo a mano deja al
  // servidor sin poder separar las partes.
  const isForm = init.body instanceof FormData;

  return fetch(`${API_URL}${path}`, {
    ...init,
    headers: {
      ...(!isForm && defaultJSONContentType ? { "Content-Type": "application/json" } : {}),
      ...(jwt ? { Authorization: `Bearer ${jwt}` } : {}),
      ...init.headers,
    },
  });
}

async function throwApiError(res: Response): Promise<never> {
  const body = await res.json().catch(() => null);
  throw new ApiError(
    res.status,
    body?.error?.code ?? "UNKNOWN",
    body?.error?.message ?? `Error ${res.status}`,
  );
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const res = await authenticatedFetch(path, init, true);

  if (res.status === 204) return undefined as T;

  if (!res.ok) {
    // Si el cuerpo no trae la forma esperada (un 502 de un proxy, por ejemplo),
    // se sintetiza un error con la misma forma para que quien llama no tenga que
    // distinguir los dos casos.
    return throwApiError(res);
  }
  const body = await res.json().catch(() => null);
  return body as T;
}

export async function apiBlob(path: string, init: RequestInit = {}): Promise<Blob> {
  const res = await authenticatedFetch(path, init, false);
  if (!res.ok) return throwApiError(res);
  return res.blob();
}

export const apiUrl = API_URL;
