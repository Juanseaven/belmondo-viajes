"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import {
  PanelRightOpen, Paperclip, Phone, Plus, Search, SendHorizontal, SlidersHorizontal,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { ConversationList, type RowState } from "./ConversationList";
import {
  DEMO_CONVERSATIONS, DEMO_LINES, DEMO_MESSAGES, DEMO_STATES, UNANSWERED,
  demoProfile,
} from "./demoData";
import { DEMO_OPERATIONS } from "./demoOperations";
import { LineFilter } from "./LineFilter";
import { MessageBubble } from "./MessageBubble";
import { OperationBanner } from "./OperationBanner";
import { OperationPanel } from "./OperationPanel";
import { operationRowState, type Control, type Operation } from "./operations";
import type { Conversation, Message } from "./api";

type InboxTab = "all" | "unanswered";

export function DemoMirrorScreen() {
  const [lineId, setLineId] = useState("");
  const [activeId, setActiveId] = useState("demo-1");
  const [tab, setTab] = useState<InboxTab>("all");
  const [search, setSearch] = useState("");
  const [operationPanelOpen, setOperationPanelOpen] = useState(true);

  // Los controles se cierran ejecutando la acción, así que tanto las
  // operaciones como los hilos tienen que poder cambiar durante la demo.
  const [operations, setOperations] = useState<Operation[]>(DEMO_OPERATIONS);
  const [threads, setThreads] = useState<Record<string, Message[]>>(DEMO_MESSAGES);

  const conversations = useMemo(() => {
    const needle = search.trim().toLocaleLowerCase("es");
    return DEMO_CONVERSATIONS.filter((conversation) => {
      if (lineId && conversation.instanceId !== lineId) return false;
      if (tab === "unanswered" && !UNANSWERED.has(conversation.id)) return false;
      return !needle || `${conversation.contactName} ${conversation.preview ?? ""}`.toLocaleLowerCase("es").includes(needle);
    });
  }, [lineId, search, tab]);

  useEffect(() => {
    if (!conversations.some((conversation) => conversation.id === activeId)) {
      setActiveId(conversations[0]?.id ?? "");
    }
  }, [activeId, conversations]);

  const conversation = DEMO_CONVERSATIONS.find((item) => item.id === activeId) ?? conversations[0];
  const line = DEMO_LINES.find((item) => item.id === conversation?.instanceId);
  const profile = conversation ? demoProfile(conversation) : null;
  const operation = operations.find((item) => item.conversationId === conversation?.id);

  /**
   * El chip de la fila no se escribe a mano cuando hay operación: sale del
   * control abierto más urgente. Es lo que un supervisor necesita ver sin
   * abrir la conversación. Las consultas sin operación conservan su estado
   * comercial, que sigue siendo cierto.
   */
  const states = useMemo(() => {
    const derived: Record<string, RowState> = { ...DEMO_STATES };
    for (const item of operations) {
      derived[item.conversationId] = {
        ...operationRowState(item),
        operation: `#${item.id} · ${item.destination}`,
      };
    }
    return derived;
  }, [operations]);

  // Un contador y no la hora: dos acciones en el mismo milisegundo repetirían la clave.
  const sent = useRef(0);

  const appendMessage = (conversationId: string, body: string, sentByName?: string): string => {
    const id = `demo-local-${(sent.current += 1)}`;
    setThreads((current) => ({
      ...current,
      [conversationId]: [...(current[conversationId] ?? []), {
        id,
        direction: "outbound",
        origin: "panel",
        type: "text",
        status: "sent",
        body,
        timestamp: new Date().toISOString(),
        sentByName,
      }],
    }));
    return id;
  };

  const closeControl = (control: Control, messageId?: string) => {
    if (!operation) return;
    setOperations((current) => current.map((item) => (
      item.id !== operation.id ? item : {
        ...item,
        controls: item.controls.map((candidate) => (
          candidate.id !== control.id ? candidate : {
            ...candidate,
            done: { at: new Date().toISOString(), by: item.seller, messageId },
          }
        )),
      }
    )));
  };

  /** Manda el mensaje y recién por eso el control queda cerrado, con la prueba. */
  const runControl = (control: Control) => {
    if (!conversation || !operation) return;
    const messageId = appendMessage(conversation.id, control.message ?? control.title, conversation.instanceLabel);
    closeControl(control, messageId);
  };

  /** Tildar a mano: queda quién y cuándo, que es lo que el papel no daba. */
  const confirmControl = (control: Control) => closeControl(control);

  /** Un checklist que no se puede corregir se llena mal una vez y queda mal. */
  const reopenControl = (control: Control) => {
    if (!operation) return;
    setOperations((current) => current.map((item) => (
      item.id !== operation.id ? item : {
        ...item,
        controls: item.controls.map((candidate) => (
          candidate.id !== control.id ? candidate : { ...candidate, done: undefined }
        )),
      }
    )));
  };

  return (
    <section className="mirror-workspace">
      <div className={`mirror-body has-details${operationPanelOpen ? "" : " operation-collapsed"}`}>
        <aside className="inbox-pane">
          {/* El filtro de línea vive arriba de la bandeja porque es la bandeja
              lo que filtra. Conectar una línea es configuración: va callado. */}
          <div className="inbox-lines">
            <LineFilter lines={DEMO_LINES} selected={lineId} onSelect={setLineId} />
            <Button type="button" variant="ghost" className="inbox-connect">
              <Plus aria-hidden />Conectar línea
            </Button>
          </div>

          <div className="inbox-tabs" role="tablist" aria-label="Filtrar conversaciones">
            <button type="button" role="tab" aria-selected={tab === "all"} onClick={() => setTab("all")}>
              Todas <span>{DEMO_CONVERSATIONS.length}</span>
            </button>
            <button type="button" role="tab" aria-selected={tab === "unanswered"} onClick={() => setTab("unanswered")}>
              Sin responder <span>{UNANSWERED.size}</span>
            </button>
          </div>

          <div className="inbox-search">
            <div>
              <Search aria-hidden />
              <Input
                type="search"
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                placeholder="Buscar contacto o mensaje…"
                aria-label="Buscar contacto o mensaje"
              />
            </div>
            <Button type="button" variant="outline" size="icon" aria-label="Más filtros">
              <SlidersHorizontal aria-hidden />
            </Button>
          </div>

          <ConversationList
            inbox={{
              items: conversations,
              loading: false,
              error: null,
              hasMore: false,
              loadMore: async () => undefined,
              reload: async () => undefined,
              applyNewMessage: () => undefined,
              applyRead: () => undefined,
            }}
            selectedId={conversation?.id ?? ""}
            onSelect={setActiveId}
            states={states}
          />
        </aside>

        <div className="thread-pane">
          {conversation && profile ? (
            <DemoThread
              key={conversation.id}
              conversation={conversation}
              operation={operation}
              messages={threads[conversation.id] ?? []}
              detailsHidden={!operationPanelOpen}
              onShowDetails={() => setOperationPanelOpen(true)}
              onSend={(body) => appendMessage(conversation.id, body, conversation.instanceLabel)}
            />
          ) : null}
        </div>

        {conversation && profile ? (
          <OperationPanel
            key={conversation.id}
            conversation={conversation}
            line={line}
            profile={profile}
            operation={operation}
            collapsed={!operationPanelOpen}
            onRunControl={runControl}
            onConfirmControl={confirmControl}
            onReopenControl={reopenControl}
            onClose={() => setOperationPanelOpen(false)}
          />
        ) : null}
      </div>
    </section>
  );
}

function DemoThread({
  conversation,
  operation,
  messages,
  detailsHidden,
  onShowDetails,
  onSend,
}: {
  conversation: Conversation;
  operation?: Operation;
  messages: Message[];
  detailsHidden: boolean;
  onShowDetails: () => void;
  onSend: (body: string) => void;
}) {
  const [body, setBody] = useState("");

  const send = () => {
    const text = body.trim();
    if (!text) return;
    onSend(text);
    setBody("");
  };

  return (
    <div className="thread">
      <header className="thread-header">
        <div className="thread-contact">
          <h2>{conversation.contactName}</h2>
          <span>
            {conversation.phoneNumber && (
              <>
                <Phone aria-hidden />
                <span className="num">{conversation.phoneNumber}</span>
                <i aria-hidden>·</i>
              </>
            )}
            <span className="thread-line">{conversation.instanceLabel}</span>
          </span>
        </div>
        {detailsHidden && (
          <Button
            type="button"
            variant={operation ? "default" : "outline"}
            size="sm"
            className={operation ? "thread-operation-trigger" : undefined}
            onClick={onShowDetails}
            aria-label={operation ? "Ver operación" : "Ver detalles"}
          >
            <PanelRightOpen aria-hidden />
            {operation ? "Ver operación" : "Ver detalles"}
          </Button>
        )}
      </header>

      {operation && <OperationBanner operation={operation} />}

      <div className="messages">
        <div className="day-divider"><span>Hoy · lunes 11 de agosto</span></div>
        {messages.map((message) => (
          <div key={message.id}><MessageBubble message={message} isGroup={conversation.isGroup} /></div>
        ))}
      </div>

      <form className="composer" onSubmit={(event) => { event.preventDefault(); send(); }}>
        <div className="composer-box">
          <Textarea
            value={body}
            onChange={(event) => setBody(event.target.value)}
            placeholder="Escribí un mensaje…"
            aria-label="Mensaje"
            rows={2}
          />
          <div className="composer-actions">
            <div className="composer-tools">
              <button type="button" style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
                <Paperclip aria-hidden />
                Adjuntar
              </button>
            </div>
            <Button type="submit" size="icon" className="composer-send" disabled={!body.trim()} aria-label="Enviar mensaje">
              <SendHorizontal aria-hidden />
            </Button>
          </div>
        </div>
      </form>
    </div>
  );
}
