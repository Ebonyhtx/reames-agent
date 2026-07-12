import { useLayoutEffect, type RefObject } from "react";

const FOCUSABLE_SELECTOR = [
  "a[href]",
  "button:not([disabled])",
  "input:not([disabled])",
  "select:not([disabled])",
  "textarea:not([disabled])",
  "[tabindex]:not([tabindex='-1'])",
].join(",");
const RETURN_FOCUS_ATTRIBUTE = "data-dialog-return-focus";

function focusableElements(dialog: HTMLElement): HTMLElement[] {
  return Array.from(dialog.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR)).filter(
    (element) => !element.hidden && element.getAttribute("aria-hidden") !== "true" && element.tabIndex >= 0,
  );
}

function topModalDialog(): HTMLElement | null {
  const dialogs = document.querySelectorAll<HTMLElement>('[aria-modal="true"]');
  return dialogs.length > 0 ? dialogs[dialogs.length - 1] : null;
}

function focusElement(element: HTMLElement | null): void {
  try {
    element?.focus();
  } catch {
    // A detached element or a limited test DOM can reject focus.
  }
}

function currentRestoreTarget(target: HTMLElement): HTMLElement | null {
  if (target.isConnected) return target;
  const key = target.getAttribute(RETURN_FOCUS_ATTRIBUTE);
  if (!key) return null;
  return Array.from(document.querySelectorAll<HTMLElement>(`[${RETURN_FOCUS_ATTRIBUTE}]`))
    .find((candidate) => candidate.getAttribute(RETURN_FOCUS_ATTRIBUTE) === key && candidate.isConnected) ?? null;
}

function restoreAfterRemoval(dialog: HTMLElement, target: HTMLElement): void {
  if (typeof MutationObserver === "undefined" || !dialog.isConnected) return;
  const observer = new MutationObserver(() => {
    if (dialog.isConnected) return;
    observer.disconnect();
    window.clearTimeout(timeout);
    const topDialog = topModalDialog();
    const currentTarget = currentRestoreTarget(target);
    if (currentTarget && (!topDialog || topDialog.contains(currentTarget))) focusElement(currentTarget);
  });
  observer.observe(document.documentElement, { childList: true, subtree: true });
  const timeout = window.setTimeout(() => observer.disconnect(), 1000);
}

// Owns focus movement and Tab containment for an aria-modal dialog. Escape
// remains with the component because closing can have domain-specific rules.
export function useDialogFocus(
  open: boolean,
  dialogRef: RefObject<HTMLElement | null>,
  initialFocusRef?: RefObject<HTMLElement | null>,
  restoreFocusRef?: RefObject<HTMLElement | null>,
): void {
  useLayoutEffect(() => {
    if (!open || typeof document === "undefined") return;

    const explicitRestoreTarget = restoreFocusRef?.current;
    const previouslyFocused = explicitRestoreTarget?.isConnected
      ? explicitRestoreTarget
      : document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    let ownedDialog: HTMLElement | null = null;

    const moveFocusInside = (): boolean => {
      const dialog = dialogRef.current;
      if (!dialog) return false;
      ownedDialog = dialog;
      if (dialog.contains(document.activeElement)) return true;
      focusElement(initialFocusRef?.current ?? focusableElements(dialog)[0] ?? dialog);
      return true;
    };
    let frame: number | null = null;
    let remainingMountFrames = 4;
    const focusWhenMounted = () => {
      frame = null;
      if (moveFocusInside() || remainingMountFrames <= 1 || typeof requestAnimationFrame !== "function") return;
      remainingMountFrames -= 1;
      frame = requestAnimationFrame(focusWhenMounted);
    };
    if (!moveFocusInside() && typeof requestAnimationFrame === "function") {
      frame = requestAnimationFrame(focusWhenMounted);
    }

    const containTab = (event: KeyboardEvent) => {
      const dialog = dialogRef.current;
      if (event.key !== "Tab" || !dialog || topModalDialog() !== dialog) return;
      const focusable = focusableElements(dialog);
      if (focusable.length === 0) {
        event.preventDefault();
        focusElement(dialog);
        return;
      }
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      const active = document.activeElement;
      if (event.shiftKey && (active === first || !dialog.contains(active))) {
        event.preventDefault();
        focusElement(last);
      } else if (!event.shiftKey && (active === last || !dialog.contains(active))) {
        event.preventDefault();
        focusElement(first);
      }
    };
    document.addEventListener("keydown", containTab, { capture: true });

    return () => {
      if (frame !== null && typeof cancelAnimationFrame === "function") cancelAnimationFrame(frame);
      document.removeEventListener("keydown", containTab, { capture: true });
      const dialog = dialogRef.current ?? ownedDialog;
      const currentTarget = previouslyFocused ? currentRestoreTarget(previouslyFocused) : null;
      if (dialog && topModalDialog() === dialog && currentTarget && previouslyFocused) {
        focusElement(currentTarget);
        restoreAfterRemoval(dialog, previouslyFocused);
      }
    };
  }, [open, dialogRef, initialFocusRef, restoreFocusRef]);
}
