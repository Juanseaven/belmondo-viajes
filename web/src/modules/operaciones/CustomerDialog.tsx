"use client";

import { useEffect, useState, type FormEvent } from "react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { formatTypedPhone } from "@/lib/phone";
import { ContactAvatar } from "@/modules/mirror/ContactAvatar";
import { conversationAvatarUrl } from "@/modules/mirror/api";
import { operacionesApi } from "./api";
import { dayOf } from "./format";
import { Field } from "./PanelEditor";
import type { Customer, CustomerMatch } from "./types";

interface Draft { name: string; phoneNumber: string; email: string; documentType: string; documentNumber: string; customerSince: string }

const fromCustomer = (customer?: Customer): Draft => ({
  name: customer?.name ?? "", phoneNumber: customer?.phoneNumber ?? "", email: customer?.email ?? "",
  documentType: customer?.documentType || "DNI", documentNumber: customer?.documentNumber ?? "",
  customerSince: customer ? dayOf(customer.customerSince) : "",
});

/**
 * El alta de quien compra sin haber escrito por WhatsApp, y la edición de un
 * cliente. Solo el nombre es obligatorio. Al dar de alta, avisa mientras se
 * escribe si se parece a un cliente que ya existe; avisa, no impide.
 */
export function CustomerDialog({ customer, onClose, onCreated, onSaved, onOpenCustomer }: {
  /** Con cliente, edita; sin él, da de alta. */
  customer?: Customer;
  onClose: () => void;
  /** `withTrip`: se eligió «Crear y armar un viaje». */
  onCreated?: (customer: Customer, withTrip: boolean) => void;
  onSaved?: (customer: Customer) => void;
  /** «Ir a su ficha» desde el aviso de repetido. */
  onOpenCustomer?: (customerId: string) => void;
}) {
  const editing = Boolean(customer);
  const [draft, setDraft] = useState<Draft>(() => fromCustomer(customer));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string>();
  const similar = useSimilar(editing ? undefined : draft);
  const set = (key: keyof Draft) => (event: { target: { value: string } }) => setDraft({ ...draft, [key]: event.target.value });

  const save = async (withTrip: boolean) => {
    if (!draft.name.trim()) {
      setError("El nombre es obligatorio");
      return;
    }
    setSaving(true);
    setError(undefined);
    const fields = {
      name: draft.name.trim(), phoneNumber: draft.phoneNumber.trim(), email: draft.email.trim(),
      documentType: draft.documentNumber.trim() ? draft.documentType : "", documentNumber: draft.documentNumber.trim(),
    };
    try {
      if (customer) {
        const saved = await operacionesApi.updateCustomer(customer.id, {
          ...fields, ...(draft.customerSince ? { customerSince: draft.customerSince } : {}),
        });
        onSaved?.(saved);
      } else {
        const created = await operacionesApi.createCustomer(fields);
        onCreated?.(created, withTrip);
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo guardar el cliente");
      setSaving(false);
    }
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="customer-dialog sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{editing ? "Editar cliente" : "Nuevo cliente"}</DialogTitle>
          <DialogDescription>Quien compra el viaje. Los pasajeros se cargan en cada viaje.</DialogDescription>
        </DialogHeader>
        <form className="customer-form" onSubmit={(event: FormEvent) => { event.preventDefault(); void save(false); }}>
          <Field label="Nombre y apellido">
            <input value={draft.name} onChange={set("name")} autoFocus required />
          </Field>
          {similar.length > 0 && (
            <div className="customer-similar" role="status">
              <b>{similar.length === 1 ? "¿Es este cliente?" : "¿Es alguno de estos clientes?"}</b>
              {similar.map((match) => (
                <div key={match.id} className="customer-similar-row">
                  <ContactAvatar name={match.name} src={match.conversationId ? conversationAvatarUrl(match.conversationId) : undefined} className="avatar-32" />
                  <span>
                    <strong>{match.name}</strong>
                    <small>{matchLine(match)}</small>
                  </span>
                  <Button type="button" size="sm" variant="outline" onClick={() => onOpenCustomer?.(match.id)}>Ir a su ficha</Button>
                </div>
              ))}
            </div>
          )}
          <div className="field-pair">
            <Field label="Teléfono"><input value={draft.phoneNumber} onChange={set("phoneNumber")} placeholder="Ej.: 11 4788-2210" inputMode="tel" /></Field>
            <Field label="Mail"><input type="email" value={draft.email} onChange={set("email")} placeholder="nombre@mail.com" /></Field>
          </div>
          <div className="customer-doc">
            <Field label="Documento">
              <select value={draft.documentType} onChange={set("documentType")}>
                <option value="DNI">DNI</option>
                <option value="Pasaporte">Pasaporte</option>
              </select>
            </Field>
            <Field label="Número"><input value={draft.documentNumber} onChange={set("documentNumber")} placeholder="Sin puntos" /></Field>
          </div>
          {editing && (
            <Field label="Cliente desde"><input type="date" value={draft.customerSince} onChange={set("customerSince")} /></Field>
          )}
          {error && <p className="panel-error" role="alert">{error}</p>}
          <DialogFooter className="customer-dialog-foot">
            {!editing && <small>Solo el nombre es obligatorio.</small>}
            <Button type="button" variant="ghost" onClick={onClose}>Cancelar</Button>
            {editing ? (
              <Button type="submit" disabled={saving}>{saving ? "Guardando…" : "Guardar cambios"}</Button>
            ) : <>
              <Button type="submit" variant="outline" disabled={saving}>Crear</Button>
              <Button type="button" disabled={saving} onClick={() => void save(true)}>Crear y armar un viaje</Button>
            </>}
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function matchLine(match: CustomerMatch) {
  const trips = match.tripCount === 0 ? "sin viajes" : `${match.tripCount} ${match.tripCount === 1 ? "viaje" : "viajes"}`;
  return [match.phoneNumber ? formatTypedPhone(match.phoneNumber) : match.email, trips, match.conversationId ? "con WhatsApp" : undefined]
    .filter(Boolean).join(" · ");
}

/** Los clientes que se parecen al que se está cargando, con una pausa corta: no por tecla. */
function useSimilar(draft?: Draft) {
  const [matches, setMatches] = useState<CustomerMatch[]>([]);
  const key = draft ? [draft.name, draft.phoneNumber, draft.email, draft.documentNumber].join("|") : "";

  useEffect(() => {
    if (!draft) return;
    const name = draft.name.trim();
    const enough = name.length >= 3 || draft.phoneNumber.trim() || draft.email.trim() || draft.documentNumber.trim();
    if (!enough) {
      setMatches([]);
      return;
    }
    let current = true;
    const timer = setTimeout(() => {
      Promise.resolve(operacionesApi.similarCustomers({
        nombre: name, telefono: draft.phoneNumber.trim(), mail: draft.email.trim(), documento: draft.documentNumber.trim(),
      }))
        .then((response) => { if (current) setMatches(response?.items ?? []); })
        .catch(() => { if (current) setMatches([]); });
    }, 350);
    return () => { current = false; clearTimeout(timer); };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);

  return matches;
}
