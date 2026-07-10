import type { DictKey, Translator } from "./i18n";
import type { WireErrorInfo } from "./types";

const ERROR_MESSAGE_KEYS: Readonly<Record<string, DictKey>> = {
  unknown: "runtimeError.unknown",
  provider_auth: "runtimeError.providerAuth",
  provider_rate_limit: "runtimeError.providerRateLimit",
  provider_server_error: "runtimeError.providerServerError",
  provider_timeout: "runtimeError.providerTimeout",
  stream_interrupted: "runtimeError.streamInterrupted",
  provider_unavailable: "runtimeError.providerUnavailable",
  tool_timeout: "runtimeError.toolTimeout",
  tool_permission: "runtimeError.toolPermission",
  tool_sandbox: "runtimeError.toolSandbox",
  tool_not_found: "runtimeError.toolNotFound",
  approval_denied: "runtimeError.approvalDenied",
  approval_timeout: "runtimeError.approvalTimeout",
  session_locked: "runtimeError.sessionLocked",
  session_not_found: "runtimeError.sessionNotFound",
  session_closed: "runtimeError.sessionClosed",
  cancelled: "runtimeError.cancelled",
};

export function runtimeErrorMessage(error: WireErrorInfo | undefined, fallback: string, t: Translator): string {
  if (!error) return fallback;
  const key = ERROR_MESSAGE_KEYS[error.code];
  return key ? t(key) : error.message || fallback || t("runtimeError.unknown");
}

export function runtimeErrorLevel(error: WireErrorInfo | undefined): "info" | "warn" {
  return error?.category === "user" || error?.category === "cancelled" ? "info" : "warn";
}

export function runtimeErrorRetryPrompt(error: WireErrorInfo | undefined, originalPrompt: string | undefined, t: Translator): string | undefined {
  if (!error?.retryable || !originalPrompt) return undefined;
  return error.code === "stream_interrupted" ? t("runtimeError.continuePrompt") : originalPrompt;
}
