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

type IsolationRecord = {
  element: HTMLElement;
  ariaHidden: string | null;
  inert: string | null;
};

type ModalLease = {
  dialog: HTMLElement;
  restoreTargets: HTMLElement[];
  released: boolean;
};

const activeModalLeases: ModalLease[] = [];
let currentIsolation: IsolationRecord[] = [];
let isolationObserver: MutationObserver | null = null;

function focusableElements(dialog: HTMLElement): HTMLElement[] {
  return Array.from(dialog.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR)).filter(
    (element) => !element.hidden && element.getAttribute("aria-hidden") !== "true" && element.tabIndex >= 0,
  );
}

function topModalDialog(): HTMLElement | null {
  for (let index = activeModalLeases.length - 1; index >= 0; index -= 1) {
    const lease = activeModalLeases[index];
    if (!lease.released && lease.dialog.isConnected && lease.dialog.getAttribute("aria-modal") === "true") return lease.dialog;
  }
  return null;
}

export function isTopModalDialog(dialog: HTMLElement | null | undefined): boolean {
  return Boolean(dialog) && topModalDialog() === dialog;
}

function restoreIsolation(): void {
  for (const record of currentIsolation) {
    if (record.ariaHidden === null) record.element.removeAttribute("aria-hidden");
    else record.element.setAttribute("aria-hidden", record.ariaHidden);
    if (record.inert === null) record.element.removeAttribute("inert");
    else record.element.setAttribute("inert", record.inert);
  }
  currentIsolation = [];
}

function recomputeIsolation(): void {
  isolationObserver?.disconnect();
  isolationObserver = null;
  restoreIsolation();
  const dialog = topModalDialog();
  if (!dialog) return;

  const records: IsolationRecord[] = [];
  const pathParents = new Set<HTMLElement>();
  let pathNode: HTMLElement | null = dialog;
  while (pathNode?.parentElement) {
    const parent: HTMLElement = pathNode.parentElement;
    pathParents.add(parent);
    for (const sibling of Array.from(parent.children)) {
      if (!(sibling instanceof HTMLElement) || sibling === pathNode) continue;
      records.push({
        element: sibling,
        ariaHidden: sibling.getAttribute("aria-hidden"),
        inert: sibling.getAttribute("inert"),
      });
      sibling.setAttribute("aria-hidden", "true");
      sibling.setAttribute("inert", "");
    }
    if (parent === dialog.ownerDocument.body) break;
    pathNode = parent;
  }
  currentIsolation = records;

  const Observer = dialog.ownerDocument.defaultView?.MutationObserver ?? (typeof MutationObserver === "undefined" ? undefined : MutationObserver);
  if (Observer) {
    isolationObserver = new Observer((mutations) => {
      if (mutations.some((mutation) => pathParents.has(mutation.target as HTMLElement))) recomputeIsolation();
    });
    isolationObserver.observe(dialog.ownerDocument.documentElement, { childList: true, subtree: true });
  }
}

function restoreTargetChain(target: HTMLElement | null): HTMLElement[] {
  if (!target) return [];
  const targets = [target];
  for (let index = activeModalLeases.length - 1; index >= 0; index -= 1) {
    const parent = activeModalLeases[index];
    if (parent.released || !parent.dialog.contains(target)) continue;
    for (const candidate of parent.restoreTargets) {
      if (!targets.includes(candidate)) targets.push(candidate);
    }
    break;
  }
  return targets;
}

function activateModalDialog(dialog: HTMLElement, restoreTargets: HTMLElement[]): ModalLease {
  const lease = { dialog, restoreTargets, released: false };
  activeModalLeases.push(lease);
  recomputeIsolation();
  return lease;
}

function deactivateModalDialog(lease: ModalLease): void {
  if (lease.released) return;
  lease.released = true;
  const index = activeModalLeases.indexOf(lease);
  if (index >= 0) activeModalLeases.splice(index, 1);
  recomputeIsolation();
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
  return Array.from(target.ownerDocument.querySelectorAll<HTMLElement>(`[${RETURN_FOCUS_ATTRIBUTE}]`))
    .find((candidate) => candidate.getAttribute(RETURN_FOCUS_ATTRIBUTE) === key && candidate.isConnected) ?? null;
}

function availableRestoreTarget(targets: HTMLElement[], topDialog: HTMLElement | null): HTMLElement | null {
  for (const target of targets) {
    const current = currentRestoreTarget(target);
    if (current && (!topDialog || topDialog.contains(current))) return current;
  }
  return null;
}

function afterDialogRemoval(dialog: HTMLElement, callback: () => void): void {
  if (!dialog.isConnected || typeof MutationObserver === "undefined") {
    callback();
    return;
  }
  let completed = false;
  const finish = () => {
    if (completed) return;
    completed = true;
    observer.disconnect();
    ownerWindow.clearTimeout(timeout);
    callback();
  };
  const observer = new MutationObserver(() => {
    if (dialog.isConnected) return;
    finish();
  });
  const ownerWindow = dialog.ownerDocument.defaultView ?? window;
  observer.observe(dialog.ownerDocument.documentElement, { childList: true, subtree: true });
  const timeout = ownerWindow.setTimeout(finish, 1000);
  ownerWindow.queueMicrotask(() => {
    if (!dialog.isConnected) finish();
  });
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
    let modalLease: ModalLease | null = null;

    const moveFocusInside = (): boolean => {
      const dialog = dialogRef.current;
      if (!dialog) return false;
      ownedDialog = dialog;
      const focusTarget = initialFocusRef?.current ?? focusableElements(dialog)[0] ?? dialog;
      if (!dialog.contains(document.activeElement)) {
        focusElement(focusTarget);
      }
      if (!modalLease) {
        modalLease = activateModalDialog(dialog, restoreTargetChain(previouslyFocused));
        // A nested dialog can mount inside a portal branch that the parent
        // modal had already made inert. Chromium rejects that first focus;
        // recomputing isolation exposes the child path, so retry once.
        if (!dialog.contains(document.activeElement)) focusElement(focusTarget);
      }
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
      const lease = modalLease;
      if (!dialog || !lease) return;
      afterDialogRemoval(dialog, () => {
        deactivateModalDialog(lease);
        const topDialog = topModalDialog();
        focusElement(availableRestoreTarget(lease.restoreTargets, topDialog));
      });
    };
  }, [open, dialogRef, initialFocusRef, restoreFocusRef]);
}
