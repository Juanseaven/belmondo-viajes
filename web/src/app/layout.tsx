import type { Metadata } from "next";
import type { ReactNode } from "react";
import { SessionProvider } from "../kernel/auth/session";
import "./globals.css";

export const metadata: Metadata = {
  title: "viajes_crm",
  description: "Panel de administración de la agencia",
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="es">
      <body>
        <SessionProvider>{children}</SessionProvider>
      </body>
    </html>
  );
}
