import type { ReactNode } from "react";
import { Phone, Smartphone, UsersRound, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { displayName, formatPhone, isPhoneLike } from "@/lib/phone";

import type { Conversation } from "./api";
import { ContactAvatar } from "./ContactAvatar";

/**
 * La ficha de un contacto que no tiene viaje abierto: sus datos a la vista, con
 * ícono, y debajo lo que la pantalla quiera ofrecer (crear un viaje).
 */
export function ContactDetails({ conversation, onClose, children }: {
  conversation: Pick<Conversation, "contactName" | "avatarUrl" | "phoneNumber" | "instanceLabel" | "isGroup">;
  onClose?: () => void;
  children?: ReactNode;
}) {
  const phone = conversation.phoneNumber && !isPhoneLike(conversation.contactName) ? formatPhone(conversation.phoneNumber) : undefined;
  return (
    <>
      <header className="panel-head">
        <ContactAvatar name={conversation.contactName} src={conversation.avatarUrl} className="avatar-40" />
        <div className="panel-title">
          <strong>{displayName(conversation.contactName)}</strong>
          <small>{conversation.isGroup ? "Grupo de WhatsApp" : "Contacto de WhatsApp"}</small>
        </div>
        {onClose && (
          <Button variant="ghost" size="icon" className="icon-button" aria-label="Ocultar viaje" title="Ocultar viaje" onClick={onClose}><X /></Button>
        )}
      </header>
      <dl className="trip-facts">
        {phone && <div className="trip-fact is-wide"><Phone aria-hidden /><dt>Teléfono</dt><dd>{phone}</dd></div>}
        <div className="trip-fact is-wide">
          {conversation.isGroup ? <UsersRound aria-hidden /> : <Smartphone aria-hidden />}
          <dt>Escribe por</dt>
          <dd>Línea {conversation.instanceLabel}</dd>
        </div>
      </dl>
      <div className="panel-body">{children}</div>
    </>
  );
}
