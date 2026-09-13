"use client";

export type ToastKind = "success" | "error" | "info";
export interface Toast {
  id: number;
  kind: ToastKind;
  text: string;
}

const styles: Record<ToastKind, string> = {
  success: "border-emerald-400/40 text-emerald-200",
  error: "border-rose-400/40 text-rose-200",
  info: "border-brass-400/40 text-brass-200",
};

export function Toasts({ toasts }: { toasts: Toast[] }) {
  return (
    <div className="pointer-events-none fixed bottom-5 left-1/2 z-50 flex -translate-x-1/2 flex-col items-center gap-2">
      {toasts.map((t) => (
        <div
          key={t.id}
          className={`glass-strong animate-rise rounded-lg border px-4 py-2.5 text-sm shadow-xl ${styles[t.kind]}`}
        >
          {t.text}
        </div>
      ))}
    </div>
  );
}
