"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { UserRound } from "lucide-react";
import { Button } from "@/components/ui/button";
import { formatTypedPhone } from "@/lib/phone";
import { operacionesApi } from "./api";
import { CustomerPickerDialog } from "./LinkDialogs";
import type { CustomerMatch } from "./types";

/**
 * De qué cliente es un chat de la bandeja. Si ya es de uno, lleva a su ficha.
 * Si todavía no es de nadie, sugiere al cliente cargado a mano que tiene el
 * mismo teléfono y ofrece vincularlo a cualquier cliente que ya exista.
 */
export function ContactCustomer({ contactId, contactName, onLinked }: {
  contactId: string;
  contactName: string;
  onLinked: () => void;
}) {
  const [state, setState] = useState<{ customerId?: string; items: CustomerMatch[] }>();
  const [picking, setPicking] = useState(false);
  const [error, setError] = useState<string>();

  const load = useCallback(async () => {
    try {
      const response = await operacionesApi.contactCustomers(contactId);
      setState(response ?? { items: [] });
    } catch {
      setState({ items: [] });
    }
  }, [contactId]);

  useEffect(() => { setState(undefined); void load(); }, [load]);

  const linked = async () => {
    setPicking(false);
    await load();
    onLinked();
  };

  const link = async (customerId: string) => {
    setError(undefined);
    try {
      await operacionesApi.linkContact(customerId, contactId);
      await linked();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo vincular el chat");
    }
  };

  if (!state) return null;
  if (state.customerId) {
    return (
      <div className="panel-edit-link">
        <Link className="link-button" href={`/clientes/${state.customerId}`}><UserRound aria-hidden />Ver ficha del cliente</Link>
      </div>
    );
  }

  return (
    <>
      {state.items.map((match) => (
        <section key={match.id} className="panel-card contact-match">
          <h3>¿Es {match.name}?</h3>
          <p className="panel-card-text">
            Es un cliente con el mismo teléfono{match.phoneNumber ? ` (${formatTypedPhone(match.phoneNumber)})` : ""}
            {" · "}{match.tripCount === 0 ? "sin viajes" : `${match.tripCount} ${match.tripCount === 1 ? "viaje" : "viajes"}`}.
          </p>
          <div className="contact-match-actions">
            <Button size="sm" onClick={() => void link(match.id)}>Vincular</Button>
          </div>
        </section>
      ))}
      {error && <p className="panel-error" role="alert">{error}</p>}
      <button type="button" className="add-button" onClick={() => setPicking(true)}><UserRound aria-hidden />Vincular a un cliente existente</button>
      {picking && (
        <CustomerPickerDialog contactId={contactId} contactName={contactName} onClose={() => setPicking(false)}
          onLinked={() => void linked()} />
      )}
    </>
  );
}
