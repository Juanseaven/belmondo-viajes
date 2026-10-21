"use client";

import { ChevronDown } from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import type { TripStatus } from "./types";

export const STATUSES: { value: TripStatus; label: string }[] = [
  { value: "consulta", label: "Consulta" },
  { value: "cotizado", label: "Cotizado" },
  { value: "confirmado", label: "Confirmado" },
  { value: "en viaje", label: "En viaje" },
  { value: "cerrado", label: "Cerrado" },
];

export const STATUS_LABEL = Object.fromEntries(STATUSES.map((s) => [s.value, s.label])) as Record<TripStatus, string>;

/** La clase del color de un estado: el mismo en el panel, en Viajes y donde aparezca. */
export const statusClass = (status: TripStatus) => `status-pill is-${status.replace(" ", "-")}`;

/** El estado como etiqueta de color. El color acompaña a la palabra, no la reemplaza. */
export function StatusPill({ status }: { status: TripStatus }) {
  return <span className={statusClass(status)}>{STATUS_LABEL[status]}</span>;
}

/** La etiqueta del estado que se toca para cambiarlo. Se guarda al elegir. */
export function StatusMenu({ status, disabled, onChange }: {
  status: TripStatus;
  disabled?: boolean;
  onChange: (status: TripStatus) => void;
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild disabled={disabled}>
        <button type="button" className={`${statusClass(status)} is-menu`} aria-label={`Estado del viaje: ${STATUS_LABEL[status]}`}>
          {STATUS_LABEL[status]}<ChevronDown aria-hidden />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" sideOffset={6}>
        <DropdownMenuRadioGroup value={status} onValueChange={(value) => onChange(value as TripStatus)}>
          {STATUSES.map((option) => (
            <DropdownMenuRadioItem key={option.value} value={option.value}>
              <span className={statusClass(option.value)}>{option.label}</span>
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
