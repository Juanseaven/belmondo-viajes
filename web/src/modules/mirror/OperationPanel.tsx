"use client";

import { useState, type ReactNode } from "react";
import { AlertTriangle, Check, ChevronRight, FileText, MessageSquare, PanelRightClose, Upload } from "lucide-react";

import { Button } from "@/components/ui/button";
import { ContactAvatar } from "./ContactAvatar";
import type { Conversation, Line } from "./api";
import type { DemoContactProfile } from "./demoData";
import {
  balanceUsd, criticalControls, documentExpires, openControls,
  totalArs,
  type Control, type Operation, type OperationDocument, type Passenger, type Service,
} from "./operations";

type Tab = "resumen" | "pax" | "servicios" | "pagos" | "documentos" | "actividad";

const TABS: Array<{ id: Tab; label: string }> = [
  { id: "resumen", label: "Resumen" },
  { id: "pax", label: "Pasajeros" },
  { id: "servicios", label: "Servicios" },
  { id: "pagos", label: "Pagos" },
  { id: "documentos", label: "Documentos" },
  { id: "actividad", label: "Actividad" },
];

const money = (amount: number) => new Intl.NumberFormat("es-AR").format(amount);
const usd = (amount: number) => `USD ${money(amount)}`;

const SHORT_DATE = new Intl.DateTimeFormat("es-AR", { day: "numeric", month: "short" });
const LONG_DATE = new Intl.DateTimeFormat("es-AR", { day: "numeric", month: "short", year: "numeric" });

const day = (iso: string) => SHORT_DATE.format(new Date(`${iso.slice(0, 10)}T12:00:00`));
const fullDay = (iso: string) => LONG_DATE.format(new Date(`${iso.slice(0, 10)}T12:00:00`));
const stamp = (iso: string) =>
  new Date(iso).toLocaleString("es-AR", { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" });

const SERVICE_LABEL: Record<Service["kind"], string> = {
  aereo: "Aéreo",
  hotel: "Hotel",
  traslado: "Traslado",
  asistencia: "Asistencia",
  excursion: "Excursión",
};

export function OperationPanel({
  conversation,
  line,
  profile,
  operation,
  collapsed = false,
  onRunControl,
  onConfirmControl,
  onReopenControl,
  onClose,
}: {
  conversation: Conversation;
  line?: Line;
  profile: DemoContactProfile;
  operation?: Operation;
  collapsed?: boolean;
  onRunControl: (control: Control) => void;
  onConfirmControl: (control: Control) => void;
  onReopenControl: (control: Control) => void;
  onClose?: () => void;
}) {
  const [tab, setTab] = useState<Tab>("resumen");

  if (!operation) {
    return (
      <aside
        className={`contact-details contact-details-panel${collapsed ? " is-collapsed" : ""}`}
        aria-label="Detalles del contacto"
        aria-hidden={collapsed || undefined}
        inert={collapsed || undefined}
      >
        <header className="operation-head">
          <div>
            <strong>Sin operación</strong>
            <small>La consulta todavía no se convirtió en un viaje</small>
          </div>
          {onClose && (
            <Button type="button" variant="ghost" size="icon-sm" className="operation-close" onClick={onClose} aria-label="Ocultar detalles">
              <PanelRightClose aria-hidden />
            </Button>
          )}
        </header>
        <div className="detail-scroll">
          <ContactSection conversation={conversation} line={line} profile={profile} />
          <NoteSection profile={profile} />
        </div>
      </aside>
    );
  }

  return (
    <aside
      className={`contact-details contact-details-panel${collapsed ? " is-collapsed" : ""}`}
      aria-label="Operación"
      aria-hidden={collapsed || undefined}
      inert={collapsed || undefined}
    >
      <header className="operation-head">
        <div>
          <strong>Operación #{operation.id}</strong>
          <small>Creada el {fullDay(operation.createdAt)} por {operation.createdBy}</small>
        </div>
        {onClose && (
          <Button type="button" variant="ghost" size="icon-sm" className="operation-close" onClick={onClose} aria-label="Ocultar operación">
            <PanelRightClose aria-hidden />
          </Button>
        )}
      </header>

      <div className="detail-tabs operation-tabs" role="tablist" aria-label="Secciones de la operación">
        {TABS.map((item) => (
          <button
            key={item.id}
            type="button"
            role="tab"
            aria-selected={tab === item.id}
            onClick={() => setTab(item.id)}
          >
            {item.label}
          </button>
        ))}
      </div>

      <div className="detail-scroll">
        {tab === "resumen" && (
          <>
            <TripSection operation={operation} />
            <ControlsSection
              operation={operation}
              onRunControl={onRunControl}
              onConfirmControl={onConfirmControl}
              onReopenControl={onReopenControl}
            />
            <ContactSection conversation={conversation} line={line} profile={profile} />
            <NoteSection profile={profile} />
          </>
        )}
        {tab === "pax" && <PassengersSection operation={operation} />}
        {tab === "servicios" && <ServicesSection operation={operation} />}
        {tab === "pagos" && <PaymentSection operation={operation} />}
        {tab === "documentos" && <DocumentsSection operation={operation} />}
        {tab === "actividad" && <ActivitySection profile={profile} />}
      </div>
    </aside>
  );
}

/**
 * Bloque plegable. El expediente completo no entra de una sola vez en la
 * columna, así que cada parte se abre cuando hace falta y el resto no estorba.
 */
function Section({
  title,
  meta,
  defaultOpen = true,
  children,
}: {
  title: string;
  meta?: string;
  defaultOpen?: boolean;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(defaultOpen);

  return (
    <section className="detail-section">
      <h3>
        <button type="button" className="section-toggle" aria-expanded={open} onClick={() => setOpen(!open)}>
          <ChevronRight aria-hidden />
          {title}
          {meta && <span className="detail-count">{meta}</span>}
        </button>
      </h3>
      {open && <div className="section-body">{children}</div>}
    </section>
  );
}

function TripSection({ operation }: { operation: Operation }) {
  const adults = operation.passengers.filter((passenger) => passenger.kind === "adulto").length;
  const minors = operation.passengers.length - adults;

  return (
    <Section title="Viaje">
      <div className="detail-rows">
        <div className="detail-row"><span>Destino</span><strong>{operation.destination}</strong></div>
        <div className="detail-row">
          <span>Fechas</span>
          <strong className="num">{day(operation.departAt)} – {fullDay(operation.returnAt)}</strong>
        </div>
        <div className="detail-row">
          <span>Pasajeros</span>
          <strong className="num">
            {adults} {adults === 1 ? "adulto" : "adultos"}
            {minors > 0 && ` + ${minors} ${minors === 1 ? "menor" : "menores"}`}
          </strong>
        </div>
        <div className="detail-row"><span>Total</span><strong className="num">{usd(operation.payment.totalUsd)}</strong></div>
        <div className="detail-row"><span>Vendedor</span><strong>{operation.seller}</strong></div>
      </div>
    </Section>
  );
}

const STAGE_LABEL: Record<Control["stage"], string> = {
  carga: "Carga de reserva",
  control: "Control de reserva",
};

/**
 * El checklist, con la forma de las dos hojas: los renglones están todos
 * siempre, controlados o no, agrupados por planilla.
 *
 * Sin barra de progreso. Un porcentaje le pide a alguien que lo lleve al 100 %
 * y esconde lo único que importa, que es si queda algo crítico sin controlar;
 * por eso el encabezado cuenta lo que falta y no lo que ya está.
 *
 * Tildar cierra el renglón a mano y deja la firma. Cuando además se puede hacer
 * la cosa desde acá —mandar el voucher, avisar por el vencimiento— la acción
 * queda a la derecha: cierra igual, pero con el mensaje como prueba.
 */
function ControlsSection({
  operation,
  onRunControl,
  onConfirmControl,
  onReopenControl,
}: {
  operation: Operation;
  onRunControl: (control: Control) => void;
  onConfirmControl: (control: Control) => void;
  onReopenControl: (control: Control) => void;
}) {
  const open = openControls(operation);
  const critical = criticalControls(operation);
  const stages: Array<Control["stage"]> = ["carga", "control"];

  return (
    <Section
      title="Controles"
      meta={open.length === 0 ? "todo controlado" : `${open.length} sin controlar`}
    >
      {/* El aviso vive fuera de las planillas: si algo crítico está adentro de
          un grupo plegado, tiene que verse igual. */}
      {critical.length > 0 && (
        <p className="control-alert">
          <AlertTriangle aria-hidden />
          {critical.length === 1
            ? "Hay un control crítico sin resolver."
            : `Hay ${critical.length} controles críticos sin resolver.`}
        </p>
      )}

      {stages.map((stage) => {
        const rows = operation.controls.filter((control) => control.stage === stage);
        if (rows.length === 0) return null;
        const checked = rows.filter((control) => control.done).length;

        return (
          <ChecklistGroup
            key={stage}
            title={STAGE_LABEL[stage]}
            checked={checked}
            total={rows.length}
          >
            {rows.map((control) => (
              <ControlRow
                key={control.id}
                control={control}
                onRunControl={onRunControl}
                onConfirmControl={onConfirmControl}
                onReopenControl={onReopenControl}
              />
            ))}
          </ChecklistGroup>
        );
      })}
    </Section>
  );
}

/** Una planilla. Arranca cerrada solo cuando no queda nada por controlar. */
function ChecklistGroup({
  title,
  checked,
  total,
  children,
}: {
  title: string;
  checked: number;
  total: number;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(checked < total);

  return (
    <div className="checklist-group">
      <h4>
        <button type="button" className="section-toggle" aria-expanded={open} onClick={() => setOpen(!open)}>
          <ChevronRight aria-hidden />
          {title}
          <span className="detail-count">
            {checked} de {total}
          </span>
        </button>
      </h4>
      {open && <ul className="checklist">{children}</ul>}
    </div>
  );
}

function ControlRow({
  control,
  onRunControl,
  onConfirmControl,
  onReopenControl,
}: {
  control: Control;
  onRunControl: (control: Control) => void;
  onConfirmControl: (control: Control) => void;
  onReopenControl: (control: Control) => void;
}) {
  const done = control.done;
  // Lo que se cerró con un mensaje no se destilda: el mensaje ya salió.
  const locked = done?.messageId !== undefined;

  return (
    <li className={[done ? "check-row done" : "check-row", control.critical && !done ? "critical" : ""].join(" ").trim()}>
      <button
        type="button"
        role="checkbox"
        aria-checked={Boolean(done)}
        // Los renglones se repiten entre las dos hojas: sin la planilla no se
        // distinguen «Destino» de «Destino».
        aria-label={`${STAGE_LABEL[control.stage]}: ${control.title}`}
        disabled={locked}
        title={locked ? "Se cerró con un mensaje: no se puede destildar" : undefined}
        onClick={() => (done ? onReopenControl(control) : onConfirmControl(control))}
      >
        {done && <Check aria-hidden size={11} strokeWidth={3} />}
      </button>

      <div className="check-copy">
        <span className="check-title">
          {control.critical && !done && <AlertTriangle aria-hidden />}
          {control.title}
        </span>

        {done ? (
          // Quién y cuándo: es lo que la planilla en papel quería y no daba.
          <small className="check-sign">
            {done.by} · {stamp(done.at)}
            {done.messageId && " · en la conversación"}
          </small>
        ) : (
          (control.owner || control.dueAt) && (
            <small className="check-meta">
              {control.owner}
              {control.owner && control.dueAt && " · "}
              {control.dueAt && <span className="num">vence {day(control.dueAt)}</span>}
            </small>
          )
        )}
      </div>

      {!done && control.closing === "accion" && (
        <button type="button" className="check-action" onClick={() => onRunControl(control)}>
          {control.actionLabel}
        </button>
      )}
    </li>
  );
}

function PassengersSection({ operation }: { operation: Operation }) {
  return (
    <section className="detail-section">
      <h3>Pasajeros <span className="detail-count">{operation.passengers.length}</span></h3>
      <ul className="pax-list">
        {operation.passengers.map((passenger) => (
          <PassengerRow key={passenger.id} passenger={passenger} returnAt={operation.returnAt} />
        ))}
      </ul>
    </section>
  );
}

function PassengerRow({ passenger, returnAt }: { passenger: Passenger; returnAt: string }) {
  const missing = passenger.document.number === "";
  const expires = passenger.document.expiresAt;
  const expired = documentExpires(passenger, returnAt);

  return (
    <li className="pax">
      <div className="pax-head">
        {/* El orden importa: la planilla controla que apellido y nombre no se inviertan. */}
        <strong>{passenger.lastName}, {passenger.firstName}</strong>
        {passenger.isHolder && <span className="chip">Titular</span>}
        {passenger.kind === "menor" && <span className="chip">Menor</span>}
      </div>
      {missing ? (
        <p className="pax-missing">Falta el documento</p>
      ) : (
        <p className="num">
          {passenger.document.type} {passenger.document.number}
          {expires && (
            <span className={expired ? "pax-expiry expired" : "pax-expiry"}>
              vence {fullDay(expires)}
            </span>
          )}
        </p>
      )}
    </li>
  );
}

/**
 * Acá vive la segunda tabla de la planilla de control. Los valores por servicio
 * son los renglones que se anotaban a mano, y el control de liquidación —que
 * los montos cotizados coincidan— pasa a ser una resta.
 */
function ServicesSection({ operation }: { operation: Operation }) {
  return (
    <section className="detail-section">
      <h3>Servicios</h3>
      <ul className="service-list">
        {operation.services.map((service) => (
          <li key={service.id} className="service">
            <div className="service-head">
              <span className="chip">{SERVICE_LABEL[service.kind]}</span>
              <strong className="num">{usd(service.amountUsd)}</strong>
            </div>
            <p>{service.detail}</p>
            <div className="service-meta">
              {service.provider && <span>{service.provider}</span>}
              {service.dates && <span className="num">{service.dates}</span>}
              {service.board && <span>{service.board}</span>}
              {service.baggage && <span>{service.baggage}</span>}
              <span className={`service-status ${service.status}`}>{service.status}</span>
            </div>
          </li>
        ))}
      </ul>

    </section>
  );
}

function PaymentSection({ operation }: { operation: Operation }) {
  const { payment } = operation;
  const balance = balanceUsd(operation);

  return (
    <section className="detail-section">
      <h3>Pagos</h3>
      <div className="detail-rows">
        <div className="detail-row"><span>Total</span><strong className="num">{usd(payment.totalUsd)}</strong></div>
        <div className="detail-row"><span>En pesos</span><strong className="num">ARS {money(totalArs(operation))}</strong></div>
        <div className="detail-row"><span>Cambio del día</span><strong className="num">{money(payment.rate)}</strong></div>
        <div className="detail-row"><span>Método</span><strong>{payment.method}</strong></div>
      </div>

      <div className="detail-rows payment-deposit">
        <div className="detail-row">
          <span>Seña</span>
          <strong className="num">
            {usd(payment.depositUsd)}
            <span className={payment.depositPaid ? "chip" : "chip warn"}>
              {payment.depositPaid ? "recibida" : "sin confirmar"}
            </span>
          </strong>
        </div>
        {payment.depositDueAt && (
          <div className="detail-row"><span>Vence</span><strong className="num">{fullDay(payment.depositDueAt)}</strong></div>
        )}
        <div className="detail-row"><span>Saldo</span><strong className="num">{usd(balance)}</strong></div>
        {payment.nonRefundableUsd !== undefined && (
          <div className="detail-row">
            <span>No reembolsable</span>
            <strong className="num">{usd(payment.nonRefundableUsd)}</strong>
          </div>
        )}
      </div>
    </section>
  );
}

function DocumentsSection({ operation }: { operation: Operation }) {
  if (operation.documents.length === 0) {
    return (
      <section className="detail-section">
        <h3>Documentos</h3>
        <p className="detail-empty">Todavía no hay documentos cargados en esta operación.</p>
      </section>
    );
  }

  return (
    <section className="detail-section">
      <h3>Documentos <span className="detail-count">{operation.documents.length}</span></h3>
      <ul className="document-list">
        {operation.documents.map((document) => (
          <li key={document.id}>
            <FileText aria-hidden />
            <div>
              <strong>{document.name}</strong>
              <DocumentSource document={document} />
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}

/** Un documento sin procedencia no prueba nada, así que siempre se dice de dónde salió. */
function DocumentSource({ document }: { document: OperationDocument }) {
  if (document.source.via === "whatsapp") {
    return (
      <small>
        <MessageSquare aria-hidden />
        Llegó por WhatsApp · {stamp(document.source.at)}
      </small>
    );
  }
  return (
    <small>
      <Upload aria-hidden />
      {document.source.by} · {stamp(document.source.at)}
    </small>
  );
}

function ContactSection({
  conversation,
  line,
  profile,
}: {
  conversation: Conversation;
  line?: Line;
  profile: DemoContactProfile;
}) {
  return (
    <>
      <section className="contact-summary">
        <ContactAvatar name={conversation.contactName} src={conversation.avatarUrl} size="lg" />
        <div>
          <strong>{conversation.contactName}</strong>
          <small>{profile.kind}</small>
        </div>
      </section>

      <Section title="Contacto" defaultOpen={false}>
        <div className="detail-rows">
          <div className="detail-row"><span>Teléfono</span><strong className="num">{conversation.phoneNumber || "—"}</strong></div>
          {/* Medio proceso administrativo sale por mail, así que el mail es un dato, no un extra. */}
          <div className="detail-row"><span>Mail</span><strong>{profile.email || "—"}</strong></div>
          <div className="detail-row"><span>Cliente desde</span><strong>{profile.since}</strong></div>
          <div className="detail-row">
            <span>Atendido por</span>
            <strong>
              <span className={`line-status-dot ${line?.status ?? "open"}`} aria-hidden />
              {line?.displayName || conversation.instanceLabel}
            </strong>
          </div>
        </div>
      </Section>
    </>
  );
}

function NoteSection({ profile }: { profile: DemoContactProfile }) {
  return (
    <Section title="Nota interna" defaultOpen={false}>
      <div className="detail-note">
        {profile.note.body.map((paragraph) => <p key={paragraph}>{paragraph}</p>)}
        {profile.note.time && <footer><time>Actualizada {profile.note.time}</time></footer>}
      </div>
    </Section>
  );
}

function ActivitySection({ profile }: { profile: DemoContactProfile }) {
  return (
    <div className="activity-feed">
      {profile.activity.map((item) => (
        <article key={`${item.title}-${item.time}`} className={item.mark ? "mark" : undefined}>
          <span aria-hidden />
          <div><strong>{item.title}</strong><p>{item.detail}</p><time>{item.time}</time></div>
        </article>
      ))}
    </div>
  );
}
