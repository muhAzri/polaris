"use client";

import type { ReactNode } from "react";

/** A row of small quiet buttons used for section and module management. */
export function ManageBar({ children }: { children: ReactNode }) {
  return <div className="flex flex-wrap items-center gap-2 text-sm">{children}</div>;
}

export function ManageButton({
  onClick,
  disabled,
  children,
  label,
}: {
  onClick: () => void;
  disabled?: boolean;
  children: ReactNode;
  label?: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      aria-label={label}
      className="btn btn-quiet btn-sm"
    >
      {children}
    </button>
  );
}
