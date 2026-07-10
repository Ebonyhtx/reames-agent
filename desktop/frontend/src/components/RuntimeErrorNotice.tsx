import { RefreshCw, Settings } from "lucide-react";
import { useT } from "../lib/i18n";
import { runtimeErrorLevel, runtimeErrorMessage, runtimeErrorRetryPrompt } from "../lib/runtimeErrors";
import type { Item } from "../lib/useController";

type NoticeItem = Extract<Item, { kind: "notice" }>;

export function RuntimeErrorNotice({
  item,
  onRetry,
  onOpenSettings,
  retryDisabled = false,
}: {
  item: NoticeItem;
  onRetry?: (text: string) => void;
  onOpenSettings?: () => void;
  retryDisabled?: boolean;
}) {
  const t = useT();
  const level = item.error ? runtimeErrorLevel(item.error) : item.level;
  const message = runtimeErrorMessage(item.error, item.text, t);
  const retryPrompt = runtimeErrorRetryPrompt(item.error, item.retryText, t);
  const authAction = item.error?.category === "auth" && onOpenSettings;
  return (
    <div
      id={item.code ? `notice-${item.code}` : undefined}
      className={`notice-line notice-line--${level}`}
      data-entrance="true"
      role={level === "warn" ? "alert" : "status"}
    >
      <span className="notice-line__icon">{level === "warn" ? "⚠ " : "ℹ "}</span>
      <span className="notice-line__text">{message}</span>
      {authAction && (
        <button
          id={`error-action-settings-${item.code ?? "auth"}`}
          type="button"
          className="notice-line__action"
          onClick={onOpenSettings}
          aria-label={t("runtimeError.openModelSettings")}
          title={t("runtimeError.openModelSettings")}
        >
          <Settings size={13} aria-hidden="true" />
          <span>{t("runtimeError.openModelSettings")}</span>
        </button>
      )}
      {retryPrompt && onRetry && (
        <button
          id={`error-action-retry-${item.code ?? "unknown"}`}
          type="button"
          className="notice-line__action"
          onClick={() => onRetry(retryPrompt)}
          disabled={retryDisabled}
          aria-label={item.code === "stream_interrupted" ? t("runtimeError.continue") : t("common.retry")}
          title={item.code === "stream_interrupted" ? t("runtimeError.continue") : t("common.retry")}
        >
          <RefreshCw size={13} aria-hidden="true" />
          <span>{item.code === "stream_interrupted" ? t("runtimeError.continue") : t("common.retry")}</span>
        </button>
      )}
    </div>
  );
}
