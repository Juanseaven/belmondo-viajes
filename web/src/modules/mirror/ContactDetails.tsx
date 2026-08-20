import {
  Clock3,
  MessageCircleMore,
  PanelRightClose,
  Phone,
  Smartphone,
  UserRound,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

import type { Conversation, Line } from "./api";
import { ContactAvatar } from "./ContactAvatar";

export function ContactDetails({
  conversation,
  line,
  mode,
  collapsed = false,
  onClose,
}: {
  conversation: Conversation | null;
  line?: Line;
  mode: "panel" | "sheet";
  collapsed?: boolean;
  onClose?: () => void;
}) {
  if (!conversation) return null;

  const lineName = line?.displayName || line?.label || conversation.instanceLabel;
  const lineStatus = line ? LINE_STATUS[line.status] : "Estado no disponible";
  const unread = conversation.unread === 0
    ? "Sin mensajes pendientes"
    : `${conversation.unread} ${conversation.unread === 1 ? "mensaje sin leer" : "mensajes sin leer"}`;

  return (
    <aside
      className={cn("contact-details", `contact-details-${mode}`, collapsed && "is-collapsed")}
      aria-label="Detalles del contacto"
      aria-hidden={collapsed || undefined}
      inert={collapsed || undefined}
    >
      <div className="contact-details-heading">
        <span>Detalles</span>
        {mode === "panel" && onClose && (
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            className="operation-close"
            onClick={onClose}
            aria-label="Ocultar detalles"
          >
            <PanelRightClose aria-hidden />
          </Button>
        )}
      </div>

      <div className="detail-scroll">
        <div className="contact-summary">
          <ContactAvatar
            name={conversation.contactName}
            src={conversation.avatarUrl}
            size="lg"
            className="contact-summary-avatar"
          />
          <div>
            <strong>{conversation.contactName}</strong>
            <small>{conversation.isGroup ? "Grupo de WhatsApp" : "Contacto de WhatsApp"}</small>
          </div>
        </div>

        <DetailSection title="Contacto">
          <DetailRow
            icon={<Phone aria-hidden />}
            label="Teléfono"
            value={conversation.phoneNumber || "No disponible"}
          />
          <DetailRow
            icon={<UserRound aria-hidden />}
            label="Tipo"
            value={conversation.isGroup ? "Grupo" : "Contacto individual"}
          />
        </DetailSection>

        <DetailSection title="Atención">
          <DetailRow
            icon={<Smartphone aria-hidden />}
            label="Línea actual"
            value={lineName}
            note={line?.phoneNumber}
          />
          <DetailRow
            icon={<span className={`line-status-dot ${line?.status ?? "close"}`} aria-hidden />}
            label="Estado de la línea"
            value={lineStatus}
          />
        </DetailSection>

        <DetailSection title="Conversación">
          <DetailRow
            icon={<MessageCircleMore aria-hidden />}
            label="Estado"
            value={conversation.needsReply ? "Espera respuesta" : "Al día"}
            tone={conversation.needsReply ? "warn" : "ok"}
          />
          <DetailRow icon={<MessageCircleMore aria-hidden />} label="Lectura" value={unread} />
          {conversation.lastMessageAt && (
            <DetailRow
              icon={<Clock3 aria-hidden />}
              label="Última actividad"
              value={formatActivity(conversation.lastMessageAt)}
            />
          )}
        </DetailSection>
      </div>
    </aside>
  );
}

const LINE_STATUS: Record<Line["status"], string> = {
  open: "Conectada",
  connecting: "Conectando",
  created: "Pendiente de vinculación",
  close: "Desconectada",
};

function DetailSection({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="detail-section contact-detail-section">
      <h3>{title}</h3>
      <dl className="contact-detail-list">{children}</dl>
    </section>
  );
}

function DetailRow({
  icon,
  label,
  value,
  note,
  tone,
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
  note?: string;
  tone?: "warn" | "ok";
}) {
  return (
    <div className="contact-detail-row">
      <span className="contact-detail-icon">{icon}</span>
      <div>
        <dt>{label}</dt>
        <dd className={tone ? `is-${tone}` : undefined}>{value}</dd>
        {note && <small>{note}</small>}
      </div>
    </div>
  );
}

function formatActivity(value: string): string {
  return new Intl.DateTimeFormat("es-AR", {
    day: "2-digit",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}
