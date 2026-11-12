/** Browser-specific lazy-module failures. Ordinary application errors must still reach the page boundary. */
export function isChunkLoadError(error: unknown): boolean {
  const message = error instanceof Error ? error.message : String(error);
  return /Failed to fetch dynamically imported module|error loading dynamically imported module|Importing a module script failed|Loading (?:CSS )?chunk .+ failed|Unable to preload CSS/i.test(message);
}

let edited = false;
let stopTracking: (() => void) | undefined;

/** Track only the fact of editing, never input values, passwords or submitted data. Conservative until reload. */
export function startEditTracking(): () => void {
  if (stopTracking) return stopTracking;
  const onEdit = (event: Event) => {
    const element = event.target;
    if (element instanceof Element && element.matches('input, textarea, select, [contenteditable="true"]')) edited = true;
  };
  document.addEventListener("input", onEdit, true);
  document.addEventListener("change", onEdit, true);
  stopTracking = () => {
    document.removeEventListener("input", onEdit, true);
    document.removeEventListener("change", onEdit, true);
    edited = false;
    stopTracking = undefined;
  };
  return stopTracking;
}

/** No automatic reloads. After any edit, explicit reload also asks before discarding possible input. */
export function requestReload(warning: string, reload = () => location.reload(), confirm = (message: string) => window.confirm(message)): boolean {
  if (edited && !confirm(warning)) return false;
  reload();
  return true;
}
