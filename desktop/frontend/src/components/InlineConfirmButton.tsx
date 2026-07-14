import { useEffect, useState } from "react";

// Row-level destructive actions should confirm in place instead of opening a
// global modal. First click arms the action, second click confirms it, and the
// adjacent Cancel button or any disabled state returns the button to normal.
export function InlineConfirmButton({
  id,
  confirmId,
  cancelId,
  label,
  confirmLabel,
  cancelLabel,
  disabled = false,
  danger = false,
  onConfirm,
}: {
  id?: string;
  confirmId?: string;
  cancelId?: string;
  label: string;
  confirmLabel: string;
  cancelLabel: string;
  disabled?: boolean;
  danger?: boolean;
  onConfirm: () => void | Promise<void>;
}) {
  const [armed, setArmed] = useState(false);

  useEffect(() => {
    if (disabled) setArmed(false);
  }, [disabled]);

  const run = async () => {
    if (!armed) {
      setArmed(true);
      return;
    }
    setArmed(false);
    await onConfirm();
  };

  return (
    <span className="inline-confirm">
      <button
        id={armed ? confirmId ?? id : id}
        className={`btn btn--small${armed && danger ? " btn--danger" : ""}`}
        disabled={disabled}
        type="button"
        onClick={run}
      >
        {armed ? confirmLabel : label}
      </button>
      {armed && (
        <button id={cancelId} className="btn btn--small" disabled={disabled} type="button" onClick={() => setArmed(false)}>
          {cancelLabel}
        </button>
      )}
    </span>
  );
}
