"use client";

import { useEffect, useState, type ComponentType, type SVGProps } from "react";

type FlagSet = Record<string, ComponentType<SVGProps<SVGSVGElement> & { title?: string }>>;

// Las banderas se dibujan con SVG porque en Windows los emojis de bandera salen
// como dos letras. Son unas 250: se cargan juntas, una vez, recién cuando
// alguna pantalla muestra la primera.
let flags: FlagSet | undefined;
let loading: Promise<FlagSet> | undefined;

function loadFlags(): Promise<FlagSet> {
  loading ??= import("country-flag-icons/react/3x2").then((module) => {
    flags = module as unknown as FlagSet;
    return flags;
  });
  return loading;
}

/** La bandera de un país por su código ISO. Mientras carga, un rectángulo gris. */
export function Flag({ code, title }: { code: string; title?: string }) {
  const [set, setSet] = useState(flags);

  useEffect(() => {
    if (set) return;
    let current = true;
    void loadFlags().then((loaded) => { if (current) setSet(loaded); }).catch(() => undefined);
    return () => { current = false; };
  }, [set]);

  const Svg = set?.[code.toUpperCase()];
  return (
    <span className="flag" role={title ? "img" : undefined} aria-label={title} aria-hidden={title ? undefined : true}>
      {Svg && <Svg aria-hidden />}
    </span>
  );
}
