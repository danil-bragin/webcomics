import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";

type ToastKind = "success" | "error" | "info";
type Toast = { id: number; kind: ToastKind; text: string };

type Ctx = { push: (kind: ToastKind, text: string) => void };

const ToastCtx = createContext<Ctx | null>(null);

let _seq = 0;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<Toast[]>([]);
  const push = useCallback((kind: ToastKind, text: string) => {
    const id = ++_seq;
    setItems((curr) => [...curr, { id, kind, text }]);
    setTimeout(() => setItems((curr) => curr.filter((t) => t.id !== id)), 3500);
  }, []);
  return (
    <ToastCtx.Provider value={{ push }}>
      {children}
      <ToastViewport items={items} onDismiss={(id) => setItems((c) => c.filter((t) => t.id !== id))} />
    </ToastCtx.Provider>
  );
}

export function useToast() {
  const ctx = useContext(ToastCtx);
  if (!ctx) throw new Error("useToast must be used inside <ToastProvider>");
  return ctx;
}

const KIND_STYLE: Record<ToastKind, string> = {
  success: "border-emerald-500/50 bg-emerald-500/10 text-emerald-200",
  error: "border-red-500/50 bg-red-500/10 text-red-200",
  info: "border-blue-500/40 bg-blue-500/10 text-blue-200",
};

function ToastViewport({ items, onDismiss }: { items: Toast[]; onDismiss: (id: number) => void }) {
  return (
    <div className="fixed bottom-4 right-4 z-50 flex flex-col gap-2 max-w-sm pointer-events-none">
      {items.map((t) => (
        <ToastItem key={t.id} t={t} onDismiss={() => onDismiss(t.id)} />
      ))}
    </div>
  );
}

function ToastItem({ t, onDismiss }: { t: Toast; onDismiss: () => void }) {
  // Slide-in: invisible for 1 frame, then animate to opacity-100. Keeps the
  // viewport DOM-stable while letting each toast fade in independently.
  const [shown, setShown] = useState(false);
  useEffect(() => {
    const r = requestAnimationFrame(() => setShown(true));
    return () => cancelAnimationFrame(r);
  }, []);
  return (
    <div
      role="status"
      className={
        "pointer-events-auto rounded border px-3 py-2 text-sm shadow-lg backdrop-blur-sm transition-all duration-200 " +
        KIND_STYLE[t.kind] +
        (shown ? " opacity-100 translate-y-0" : " opacity-0 translate-y-2")
      }
    >
      <div className="flex items-start gap-2">
        <span className="flex-1">{t.text}</span>
        <button onClick={onDismiss} className="text-xs opacity-60 hover:opacity-100" aria-label="dismiss">
          ×
        </button>
      </div>
    </div>
  );
}
