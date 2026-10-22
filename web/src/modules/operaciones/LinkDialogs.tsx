"use client";

import { useEffect, useState } from "react";
import { Search } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { displayName, formatPhone, formatTypedPhone } from "@/lib/phone";
import { ContactAvatar } from "@/modules/mirror/ContactAvatar";
import { conversationAvatarUrl } from "@/modules/mirror/api";
import { operacionesApi } from "./api";
import type { CustomerRow, FreeContact } from "./types";

/**
 * Vincular un chat a un cliente cargado a mano, desde su página. Busca entre
 * los chats que todavía no son de ningún cliente; primero los que coinciden con
 * su teléfono.
 */
export function LinkChatDialog({ customerId, customerName, onClose, onLinked }: {
  customerId: string;
  customerName: string;
  onClose: () => void;
  onLinked: () => void;
}) {
  const [q, setQ] = useState("");
  const results = useSearch(q, (text) => operacionesApi.freeContacts(text, customerId));
  const link = useLink(onLinked);

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="link-dialog sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Vincular un chat</DialogTitle>
          <DialogDescription>Elegí el chat de {customerName}. Solo aparecen los que todavía no son de ningún cliente.</DialogDescription>
        </DialogHeader>
        <SearchBox value={q} onChange={setQ} placeholder="Buscar por nombre o teléfono" />
        <ul className="link-list">
          {results.items.map((contact: FreeContact) => (
            <li key={contact.contactId} className="link-row">
              <ContactAvatar name={contact.name} src={conversationAvatarUrl(contact.conversationId)} className="avatar-36" />
              <span>
                <strong>{displayName(contact.name)}</strong>
                <small>{[contact.phoneNumber && formatPhone(contact.phoneNumber), contact.instanceLabel && `Línea ${contact.instanceLabel}`].filter(Boolean).join(" · ")}</small>
              </span>
              <Button size="sm" variant="outline" disabled={link.busy} onClick={() => void link.run(customerId, contact.contactId)}>Vincular</Button>
            </li>
          ))}
          {results.loaded && results.items.length === 0 && <li className="link-empty">{q.trim() ? "Ningún chat libre coincide." : "No hay chats sin cliente."}</li>}
        </ul>
        {link.error && <p className="panel-error" role="alert">{link.error}</p>}
      </DialogContent>
    </Dialog>
  );
}

/** Vincular un chat de la bandeja a un cliente que ya existe. */
export function CustomerPickerDialog({ contactId, contactName, onClose, onLinked }: {
  contactId: string;
  contactName: string;
  onClose: () => void;
  onLinked: () => void;
}) {
  const [q, setQ] = useState("");
  const results = useSearch(q, async (text) => operacionesApi.customers({ q: text }));
  const link = useLink(onLinked);

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="link-dialog sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Vincular a un cliente existente</DialogTitle>
          <DialogDescription>El chat de {displayName(contactName)} va a mostrar los viajes del cliente que elijas.</DialogDescription>
        </DialogHeader>
        <SearchBox value={q} onChange={setQ} placeholder="Buscar por nombre, teléfono, mail o documento" />
        <ul className="link-list">
          {results.items.map((customer: CustomerRow) => (
            <li key={customer.id} className="link-row">
              <ContactAvatar name={customer.name} src={customer.conversationId ? conversationAvatarUrl(customer.conversationId) : undefined} className="avatar-36" />
              <span>
                <strong>{customer.name}</strong>
                <small>{[customer.phoneNumber && formatTypedPhone(customer.phoneNumber),
                  customer.tripCount === 0 ? "sin viajes" : `${customer.tripCount} ${customer.tripCount === 1 ? "viaje" : "viajes"}`].filter(Boolean).join(" · ")}</small>
              </span>
              <Button size="sm" variant="outline" disabled={link.busy} onClick={() => void link.run(customer.id, contactId)}>Vincular</Button>
            </li>
          ))}
          {results.loaded && results.items.length === 0 && <li className="link-empty">Ningún cliente coincide.</li>}
        </ul>
        {link.error && <p className="panel-error" role="alert">{link.error}</p>}
      </DialogContent>
    </Dialog>
  );
}

function SearchBox({ value, onChange, placeholder }: { value: string; onChange: (value: string) => void; placeholder: string }) {
  return (
    <div className="trips-search link-search">
      <Search aria-hidden />
      <Input type="search" aria-label={placeholder} placeholder={placeholder} value={value} autoFocus
        onChange={(event) => onChange(event.target.value)} />
    </div>
  );
}

/** Una búsqueda que se tipea: espera un instante para no consultar por letra. */
function useSearch<T>(q: string, fetch: (text: string) => Promise<{ items: T[] }>) {
  const [items, setItems] = useState<T[]>([]);
  const [loaded, setLoaded] = useState(false);
  useEffect(() => {
    let current = true;
    const timer = setTimeout(() => {
      Promise.resolve(fetch(q.trim()))
        .then((response) => { if (current) setItems(response?.items ?? []); })
        .catch(() => { if (current) setItems([]); })
        .finally(() => { if (current) setLoaded(true); });
    }, 250);
    return () => { current = false; clearTimeout(timer); };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [q]);
  return { items, loaded };
}

function useLink(onLinked: () => void) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const run = async (customerId: string, contactId: string) => {
    setBusy(true);
    setError(undefined);
    try {
      await operacionesApi.linkContact(customerId, contactId);
      onLinked();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo vincular el chat");
    } finally {
      setBusy(false);
    }
  };
  return { busy, error, run };
}
