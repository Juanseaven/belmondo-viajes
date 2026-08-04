"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import { api, ApiError, token } from "../api/client";

export interface User {
  id: string;
  email: string;
  fullName: string;
  role: "admin" | "member";
  isActive: boolean;
}

interface Me {
  user: User;
  /** Nombres de los módulos que este usuario puede ver. El backend manda. */
  modules: string[];
}

interface Session {
  user: User | null;
  modules: string[];
  loading: boolean;
  login: (email: string, password: string) => Promise<void>;
  logout: () => void;
}

const SessionContext = createContext<Session | null>(null);

export function SessionProvider({ children }: { children: ReactNode }) {
  const [me, setMe] = useState<Me | null>(null);
  const [loading, setLoading] = useState(true);

  /**
   * Al cargar se pregunta por `/api/me` en vez de confiar en el token guardado:
   * un token puede estar bien firmado y el usuario haber sido dado de baja.
   * Quien decide si la sesión vale es el backend.
   */
  const refresh = useCallback(async () => {
    if (!token.get()) {
      setMe(null);
      setLoading(false);
      return;
    }
    try {
      setMe(await api<Me>("/api/me"));
    } catch (err) {
      if (err instanceof ApiError && err.isAuthError) token.clear();
      setMe(null);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const login = useCallback(
    async (email: string, password: string) => {
      const res = await api<{ token: string }>("/api/auth/login", {
        method: "POST",
        body: JSON.stringify({ email, password }),
      });
      token.set(res.token);
      setLoading(true);
      await refresh();
    },
    [refresh],
  );

  const logout = useCallback(() => {
    token.clear();
    setMe(null);
    void api("/api/auth/logout", { method: "POST" }).catch(() => {});
  }, []);

  return (
    <SessionContext.Provider
      value={{
        user: me?.user ?? null,
        modules: me?.modules ?? [],
        loading,
        login,
        logout,
      }}
    >
      {children}
    </SessionContext.Provider>
  );
}

export function useSession(): Session {
  const ctx = useContext(SessionContext);
  if (!ctx) throw new Error("useSession debe usarse dentro de <SessionProvider>");
  return ctx;
}
