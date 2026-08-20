import type { Metadata } from "next";
import type { ReactNode } from "react";
// Sin webfonts: la tipografía es la misma pila del sistema que declara
// WhatsApp Web (ver --font-ui en globals.css).
import { TooltipProvider } from "@/components/ui/tooltip";
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
        <TooltipProvider>
          <SessionProvider>{children}</SessionProvider>
        </TooltipProvider>
      </body>
    </html>
  );
}
