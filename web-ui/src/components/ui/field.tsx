import { type ReactNode } from "react";

/**
 * Field — small uppercase label + child input. Used in dense form layouts
 * (project defaults, timeline editor, run detail).
 */
export function Field({ label, hint, children }: { label: string; hint?: ReactNode; children: ReactNode }) {
  return (
    <label className="block">
      <span className="text-[10px] uppercase tracking-wide text-muted-foreground mb-1 block">{label}</span>
      {children}
      {hint ? <span className="text-[11px] text-muted-foreground mt-1 block">{hint}</span> : null}
    </label>
  );
}
