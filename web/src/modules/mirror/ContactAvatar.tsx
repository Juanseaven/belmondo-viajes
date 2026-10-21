"use client";

import { useEffect, useState } from "react";

import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { apiBlob } from "@/kernel/api/client";
import { cn } from "@/lib/utils";

function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return "?";
  return `${parts[0][0]}${parts.length > 1 ? parts.at(-1)?.[0] ?? "" : ""}`.toUpperCase();
}

export function ContactAvatar({
  name,
  src,
  size = "default",
  className,
}: {
  name: string;
  src?: string;
  size?: "sm" | "default" | "lg";
  className?: string;
}) {
  const internal = src?.startsWith("/api/") ?? false;
  const [loaded, setLoaded] = useState<{ source: string; url: string }>();
  const resolvedSrc = internal ? (loaded && loaded.source === src ? loaded.url : undefined) : src;

  useEffect(() => {
    let active = true;
    let objectURL: string | undefined;

    if (!src || !src.startsWith("/api/")) {
      setLoaded(undefined);
      return () => {
        active = false;
      };
    }

    void apiBlob(src)
      .then((blob) => {
        if (!active) return;
        objectURL = URL.createObjectURL(blob);
        setLoaded({ source: src, url: objectURL });
      })
      .catch(() => {
        if (active) setLoaded(undefined);
      });

    return () => {
      active = false;
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  }, [src]);

  return (
    <Avatar size={size} className={cn("contact-avatar", className)}>
      {resolvedSrc && <AvatarImage src={resolvedSrc} alt={name} />}
      <AvatarFallback>{initials(name)}</AvatarFallback>
    </Avatar>
  );
}
