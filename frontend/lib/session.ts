// A session id is created once per browser tab and kept in sessionStorage, so
// each tab is a distinct "person". Open two tabs and you have two competitors —
// that's the zero-setup way to see the race condition for yourself.
const KEY = "grabseat_session_id";

export function getSessionId(): string {
  if (typeof window === "undefined") return "";
  let id = window.sessionStorage.getItem(KEY);
  if (!id) {
    const rand =
      typeof crypto !== "undefined" && "randomUUID" in crypto
        ? crypto.randomUUID()
        : Math.random().toString(36).slice(2) + Date.now().toString(36);
    id = "tab_" + rand;
    window.sessionStorage.setItem(KEY, id);
  }
  return id;
}

// A short, friendly label for showing the session in the UI.
export function shortSession(id: string): string {
  if (!id) return "…";
  const tail = id.replace(/^tab_/, "");
  return tail.slice(0, 4).toUpperCase();
}
