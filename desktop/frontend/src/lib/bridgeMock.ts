// Browser-only development mock for the Desktop bridge.
// Loaded dynamically only when Wails bindings are unavailable.

import type { AppBindings } from "./bridge";
import { t } from "./i18n";
import { providerRequiresKey } from "./providerModels";
import { DEFAULT_STATUS_BAR_ITEMS, normalizeStatusBarItems } from "./statusBarItems";
import { modeHasAutoApproveTools, modeWithAutoApproveTools, modeWithPlan, normalizeCollaborationMode, normalizeMode, normalizeTokenMode, normalizeToolApprovalMode } from "./types";

import type {
  BotSettingsView, DesktopStartupSettingsView, HistoryMessage, HistoryPage, HookConfigView,
  HooksSettingsView, MCPServerInput, MemorySuggestion, Mode, NetworkView, PluginInstallOptions,
  PluginOperationView, PluginView, ProjectNode, PromptHistoryEntry, ProviderPresetView, ProviderView, ServerView,
  SessionMeta, SettingsView, SkillRootView, SkillSuggestion, SkillView, TabMeta, ToolApprovalMode,
  UpdateProgress, WireEvent,
} from "./types";

const EVENT_CHANNEL = "agent:event";
const GLOBAL_PROJECT_ORDER_KEY = "__global__";

function stripGoalResearchFlags(arg: string): string {
  const parts = arg.trim().split(/\s+/).filter(Boolean);
  while (parts.length > 0) {
    const flag = parts[0].toLowerCase();
    if (flag !== "--research" && flag !== "--auto-research" && flag !== "--deep" && flag !== "--simple" && flag !== "--no-research") break;
    parts.shift();
  }
  return parts.join(" ");
}
// --- browser dev mock --------------------------------------------------------

const listeners = new Set<(e: WireEvent) => void>();
let mockScopedTabId: string | undefined;

export function mockSubscribe(cb: (e: WireEvent) => void): () => void {
  listeners.add(cb);
  return () => {
    listeners.delete(cb);
  };
}

function emit(e: WireEvent) {
  const event = mockScopedTabId && !e.tabId ? { ...e, tabId: mockScopedTabId } : e;
  listeners.forEach((l) => l(event));
}

export function mockToolApprovalModeAfterModeChange(current: string | undefined, nextMode: Mode): ToolApprovalMode {
  if (modeHasAutoApproveTools(nextMode)) return "yolo";
  const currentMode = normalizeToolApprovalMode(current);
  return currentMode === "yolo" ? "ask" : currentMode;
}

async function withMockTabScope<T>(tabId: string, fn: () => Promise<T>): Promise<T> {
  const previous = mockScopedTabId;
  mockScopedTabId = tabId || previous;
  try {
    return await fn();
  } finally {
    mockScopedTabId = previous;
  }
}

// Updater progress has its own listener set so the browser dev mock can stream a
// fake download/install flow through onUpdaterProgress.
const updaterListeners = new Set<(p: UpdateProgress) => void>();

export function mockUpdaterSubscribe(cb: (p: UpdateProgress) => void): () => void {
  updaterListeners.add(cb);
  return () => updaterListeners.delete(cb);
}

function emitUpdater(p: UpdateProgress) {
  updaterListeners.forEach((l) => l(p));
}

function delay(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}

function baseName(path: string): string {
  return path.replace(/[/\\]+$/, "").split(/[/\\]/).filter(Boolean).pop() ?? path;
}

function browserPlatformOverride(): "darwin" | "windows" | "linux" | "" {
  if (typeof window === "undefined" || window.runtime) return "";
  const value = new URLSearchParams(window.location.search).get("platform");
  return value === "darwin" || value === "windows" || value === "linux" ? value : "";
}

function mockScenario(): "demo" | "fresh" | "running" | "guidance" | "sandbox_escape" {
  if (typeof window === "undefined") return "demo";
  const value = new URLSearchParams(window.location.search).get("mock")?.trim().toLowerCase();
  if (value === "fresh" || value === "empty" || value === "first-run") return "fresh";
  if (value === "guidance" || value === "guide" || value === "steer") return "guidance";
  if (value === "running" || value === "busy" || value === "streaming") return "running";
  if (value === "sandbox_escape" || value === "sandbox-escape" || value === "sandboxescape") return "sandbox_escape";
  return "demo";
}

type MockProviderPresetTemplate = {
  id: string;
  label: string;
  description: string;
  keyEnv: string;
  provider: ProviderView;
};

function mockProviderTemplate(p: Pick<ProviderView, "name" | "kind" | "baseUrl" | "models" | "default" | "apiKeyEnv"> & Partial<ProviderView>): ProviderView {
  return {
    name: p.name,
    builtIn: false,
    added: true,
    kind: p.kind,
    baseUrl: p.baseUrl,
    modelsUrl: p.modelsUrl ?? "",
    models: p.models,
    visionModels: p.visionModels ?? [],
    visionModelsConfigured: Boolean(p.visionModelsConfigured ?? ((p.visionModels ?? []).length > 0)),
    default: p.default,
    apiKeyEnv: p.apiKeyEnv,
    headers: p.headers,
    extraBody: p.extraBody,
    authHeader: p.authHeader,
    keySet: Boolean(p.keySet),
    balanceUrl: p.balanceUrl ?? "",
    contextWindow: p.contextWindow ?? 0,
    reasoningProtocol: p.reasoningProtocol ?? "",
    thinking: p.thinking ?? "",
    supportedEfforts: p.supportedEfforts ?? [],
    defaultEffort: p.defaultEffort ?? "",
    modelOverrides: p.modelOverrides,
  };
}

function mockPreset(id: string, label: string, description: string, keyEnv: string, provider: ProviderView): MockProviderPresetTemplate {
  return { id, label, description, keyEnv, provider };
}

const mockKimiAPIModels = ["kimi-k2.7-code", "kimi-k2.7-code-highspeed", "kimi-k2.6", "kimi-k2.5"];
const mockLongCatModels = ["LongCat-2.0"];
const mockMiMoV25Models = ["mimo-v2.5-pro", "mimo-v2.5"];
const mockMiniMaxModels = ["MiniMax-M3", "MiniMax-M2.7", "MiniMax-M2.7-highspeed"];
const mockGLMAPIModels = ["glm-5.2", "glm-5.1", "glm-5", "glm-5-turbo", "glm-5v-turbo", "glm-4.7", "glm-4.7-flash", "glm-4.7-flashx", "glm-4.6", "glm-4.5", "glm-4.5-air", "glm-4.5-flash"];
const mockGLMCodingModels = ["glm-5.2", "glm-5.1", "glm-5", "glm-4.7"];
const mockGLMAnthropicModels = ["glm-5.2[1m]", "glm-5.2", "glm-5.1", "glm-5", "glm-4.7", "glm-4.5-air"];
const mockQwenAPIModels = ["qwen3.7-plus", "qwen3.7-max", "qwen3.6-plus", "qwen3.5-plus", "qwen3-max-2026-01-23", "qwen3-coder-next", "qwen3-coder-plus", "MiniMax-M2.5", "glm-5", "glm-4.7", "kimi-k2.5"];
const mockQwenPlanModels = ["qwen3.7-plus", "qwen3.6-plus", "kimi-k2.5", "glm-5", "MiniMax-M2.5", "qwen3.5-plus", "qwen3-max-2026-01-23", "qwen3-coder-next", "qwen3-coder-plus", "glm-4.7"];
const mockQwenPlanVisionModels = ["qwen3.7-plus", "qwen3.6-plus", "qwen3.5-plus", "kimi-k2.5"];
const mockStepFunModels = ["step-3.7-flash", "step-3.5-flash", "step-3.5-flash-2603"];
const mockOpenCodeGoModels = ["glm-5.2", "glm-5.1", "kimi-k2.7-code", "kimi-k2.6", "deepseek-v4-pro", "deepseek-v4-flash", "mimo-v2.5-pro", "mimo-v2.5"];
const mockOpenCodeGoAnthropicModels = ["qwen3.7-plus", "qwen3.7-max", "qwen3.6-plus", "minimax-m3", "minimax-m2.7", "minimax-m2.5"];
const mockOpenCodeZenAnthropicModels = ["claude-sonnet-4-6", "claude-opus-4-8", "claude-haiku-4-5", "qwen3.6-plus", "qwen3.5-plus", "qwen3.6-plus-free"];
const mockNovitaModels = ["zai-org/glm-5.2", "moonshotai/kimi-k2.7-code", "minimax/minimax-m3", "deepseek/deepseek-v4-pro", "deepseek/deepseek-v4-flash", "qwen/qwen3.7-max", "qwen/qwen3.6-plus", "zai-org/glm-5v-turbo"];
const mockGMIModels = ["zai-org/GLM-5.2-FP8", "deepseek-ai/DeepSeek-V4-Pro", "deepseek-ai/DeepSeek-V4-Flash", "moonshotai/Kimi-K2.7-Code", "anthropic/claude-sonnet-4.6", "openai/gpt-5.5"];
const mockVercelModels = ["anthropic/claude-sonnet-4.6", "anthropic/claude-opus-4.8", "openai/gpt-5.4", "openai/gpt-5.4-pro", "moonshotai/kimi-k2.7-code", "zai/glm-5.2", "deepseek/deepseek-v4-pro"];
const mockOllamaCloudModels = ["glm-5.2", "kimi-k2.7-code", "deepseek-v4-pro", "deepseek-v4-flash", "minimax-m3", "nemotron-3-nano:30b", "qwen3-coder-next"];

const mockProviderPresetTemplates: MockProviderPresetTemplate[] = [
  mockPreset("longcat-openai", "LongCat OpenAI", "LongCat Platform OpenAI-compatible endpoint for LongCat-2.0.", "LONGCAT_API_KEY", mockProviderTemplate({ name: "longcat-openai", kind: "openai", baseUrl: "https://api.longcat.chat/openai/v1", modelsUrl: "https://api.longcat.chat/openai/v1/models", models: mockLongCatModels, default: "LongCat-2.0", apiKeyEnv: "LONGCAT_API_KEY", contextWindow: 131072, thinking: "enabled", supportedEfforts: ["enabled", "disabled"], defaultEffort: "enabled" })),
  mockPreset("longcat-anthropic", "LongCat Anthropic", "LongCat Platform Anthropic-compatible Messages endpoint for LongCat-2.0.", "LONGCAT_API_KEY", mockProviderTemplate({ name: "longcat-anthropic", kind: "anthropic", baseUrl: "https://api.longcat.chat/anthropic", modelsUrl: "https://api.longcat.chat/anthropic/v1/models", models: mockLongCatModels, default: "LongCat-2.0", apiKeyEnv: "LONGCAT_API_KEY", authHeader: true, contextWindow: 131072, thinking: "enabled", supportedEfforts: ["enabled", "disabled"], defaultEffort: "enabled" })),
  mockPreset("kimi-cn", "Kimi CN API", "Moonshot Kimi China OpenAI-compatible API.", "KIMI_API_KEY", mockProviderTemplate({ name: "kimi-cn", kind: "openai", baseUrl: "https://api.moonshot.cn/v1", models: mockKimiAPIModels, visionModels: mockKimiAPIModels, default: "kimi-k2.7-code", apiKeyEnv: "KIMI_API_KEY", balanceUrl: "https://api.moonshot.cn/v1/users/me/balance", contextWindow: 262144, reasoningProtocol: "none" })),
  mockPreset("kimi-global", "Kimi Global API", "Moonshot Kimi international OpenAI-compatible API.", "MOONSHOT_API_KEY", mockProviderTemplate({ name: "kimi-global", kind: "openai", baseUrl: "https://api.moonshot.ai/v1", models: mockKimiAPIModels, visionModels: mockKimiAPIModels, default: "kimi-k2.7-code", apiKeyEnv: "MOONSHOT_API_KEY", balanceUrl: "https://api.moonshot.ai/v1/users/me/balance", contextWindow: 262144, reasoningProtocol: "none" })),
  mockPreset("kimi-coding-plan", "Kimi Coding Plan", "Kimi Coding Plan via its dedicated Anthropic-compatible endpoint.", "KIMI_CODING_API_KEY", mockProviderTemplate({ name: "kimi-coding-plan", kind: "anthropic", baseUrl: "https://api.kimi.com/coding/", models: ["kimi-for-coding"], visionModels: ["kimi-for-coding"], default: "kimi-for-coding", apiKeyEnv: "KIMI_CODING_API_KEY", headers: { "User-Agent": "claude-code/0.1.0" }, thinking: "adaptive", contextWindow: 262144 })),
  mockPreset("mimo-api", "MiMo API", "Xiaomi MiMo direct API with text and vision-capable models.", "MIMO_API_KEY", mockProviderTemplate({ name: "mimo-api", kind: "openai", baseUrl: "https://api.xiaomimimo.com/v1", models: mockMiMoV25Models, visionModels: ["mimo-v2.5"], default: "mimo-v2.5-pro", apiKeyEnv: "MIMO_API_KEY", contextWindow: 1048576 })),
  mockPreset("mimo-anthropic", "MiMo Anthropic", "Xiaomi MiMo direct Anthropic-compatible endpoint.", "MIMO_API_KEY", mockProviderTemplate({ name: "mimo-anthropic", kind: "anthropic", baseUrl: "https://api.xiaomimimo.com/anthropic", models: mockMiMoV25Models, visionModels: ["mimo-v2.5"], default: "mimo-v2.5-pro", apiKeyEnv: "MIMO_API_KEY", thinking: "adaptive", contextWindow: 1048576 })),
  mockPreset("mimo-token-plan-cn", "MiMo Token Plan CN", "Xiaomi MiMo token-plan China endpoint.", "MIMO_TOKEN_PLAN_API_KEY", mockProviderTemplate({ name: "mimo-token-plan-cn", kind: "openai", baseUrl: "https://token-plan-cn.xiaomimimo.com/v1", models: mockMiMoV25Models, visionModels: ["mimo-v2.5"], default: "mimo-v2.5-pro", apiKeyEnv: "MIMO_TOKEN_PLAN_API_KEY", contextWindow: 1048576 })),
  mockPreset("mimo-token-plan-cn-anthropic", "MiMo Token Plan CN Anthropic", "Xiaomi MiMo token-plan China Anthropic-compatible endpoint.", "MIMO_TOKEN_PLAN_API_KEY", mockProviderTemplate({ name: "mimo-token-plan-cn-anthropic", kind: "anthropic", baseUrl: "https://token-plan-cn.xiaomimimo.com/anthropic", models: mockMiMoV25Models, visionModels: ["mimo-v2.5"], default: "mimo-v2.5-pro", apiKeyEnv: "MIMO_TOKEN_PLAN_API_KEY", thinking: "adaptive", contextWindow: 1048576 })),
  mockPreset("mimo-token-plan-sgp", "MiMo Token Plan SGP", "Xiaomi MiMo token-plan Singapore endpoint.", "MIMO_TOKEN_PLAN_API_KEY", mockProviderTemplate({ name: "mimo-token-plan-sgp", kind: "openai", baseUrl: "https://token-plan-sgp.xiaomimimo.com/v1", models: mockMiMoV25Models, visionModels: ["mimo-v2.5"], default: "mimo-v2.5-pro", apiKeyEnv: "MIMO_TOKEN_PLAN_API_KEY", contextWindow: 1048576 })),
  mockPreset("mimo-token-plan-sgp-anthropic", "MiMo Token Plan SGP Anthropic", "Xiaomi MiMo token-plan Singapore Anthropic-compatible endpoint.", "MIMO_TOKEN_PLAN_API_KEY", mockProviderTemplate({ name: "mimo-token-plan-sgp-anthropic", kind: "anthropic", baseUrl: "https://token-plan-sgp.xiaomimimo.com/anthropic", models: mockMiMoV25Models, visionModels: ["mimo-v2.5"], default: "mimo-v2.5-pro", apiKeyEnv: "MIMO_TOKEN_PLAN_API_KEY", thinking: "adaptive", contextWindow: 1048576 })),
  mockPreset("mimo-token-plan-ams", "MiMo Token Plan AMS", "Xiaomi MiMo token-plan Amsterdam endpoint.", "MIMO_TOKEN_PLAN_API_KEY", mockProviderTemplate({ name: "mimo-token-plan-ams", kind: "openai", baseUrl: "https://token-plan-ams.xiaomimimo.com/v1", models: mockMiMoV25Models, visionModels: ["mimo-v2.5"], default: "mimo-v2.5-pro", apiKeyEnv: "MIMO_TOKEN_PLAN_API_KEY", contextWindow: 1048576 })),
  mockPreset("mimo-token-plan-ams-anthropic", "MiMo Token Plan AMS Anthropic", "Xiaomi MiMo token-plan Amsterdam Anthropic-compatible endpoint.", "MIMO_TOKEN_PLAN_API_KEY", mockProviderTemplate({ name: "mimo-token-plan-ams-anthropic", kind: "anthropic", baseUrl: "https://token-plan-ams.xiaomimimo.com/anthropic", models: mockMiMoV25Models, visionModels: ["mimo-v2.5"], default: "mimo-v2.5-pro", apiKeyEnv: "MIMO_TOKEN_PLAN_API_KEY", thinking: "adaptive", contextWindow: 1048576 })),
  mockPreset("minimax-cn-api", "MiniMax CN API", "MiniMax China OpenAI-compatible M-series API endpoint.", "MINIMAX_API_KEY", mockProviderTemplate({ name: "minimax-cn-api", kind: "openai", baseUrl: "https://api.minimaxi.com/v1", models: mockMiniMaxModels, visionModels: ["MiniMax-M3"], default: "MiniMax-M3", apiKeyEnv: "MINIMAX_API_KEY", extraBody: { reasoning_split: true }, contextWindow: 1048576, thinking: "adaptive", supportedEfforts: ["disabled", "adaptive"], defaultEffort: "adaptive" })),
  mockPreset("minimax-global-api", "MiniMax Global API", "MiniMax international OpenAI-compatible M-series API endpoint.", "MINIMAX_API_KEY", mockProviderTemplate({ name: "minimax-global-api", kind: "openai", baseUrl: "https://api.minimax.io/v1", models: mockMiniMaxModels, visionModels: ["MiniMax-M3"], default: "MiniMax-M3", apiKeyEnv: "MINIMAX_API_KEY", extraBody: { reasoning_split: true }, contextWindow: 1048576, thinking: "adaptive", supportedEfforts: ["disabled", "adaptive"], defaultEffort: "adaptive" })),
  mockPreset("minimax-cn-anthropic", "MiniMax CN Anthropic", "MiniMax China Anthropic-compatible M-series endpoint.", "MINIMAX_PLAN_API_KEY", mockProviderTemplate({ name: "minimax-cn-anthropic", kind: "anthropic", baseUrl: "https://api.minimaxi.com/anthropic", models: mockMiniMaxModels, visionModels: ["MiniMax-M3"], default: "MiniMax-M3", apiKeyEnv: "MINIMAX_PLAN_API_KEY", authHeader: true, contextWindow: 1048576, thinking: "adaptive", supportedEfforts: ["disabled", "adaptive"], defaultEffort: "adaptive" })),
  mockPreset("minimax-global-anthropic", "MiniMax Global Anthropic", "MiniMax international Anthropic-compatible endpoint with Bearer auth.", "MINIMAX_API_KEY", mockProviderTemplate({ name: "minimax-global-anthropic", kind: "anthropic", baseUrl: "https://api.minimax.io/anthropic", models: mockMiniMaxModels, visionModels: ["MiniMax-M3"], default: "MiniMax-M3", apiKeyEnv: "MINIMAX_API_KEY", authHeader: true, contextWindow: 1048576, thinking: "adaptive", supportedEfforts: ["disabled", "adaptive"], defaultEffort: "adaptive" })),
  mockPreset("glm-cn", "GLM CN API", "Zhipu GLM China OpenAI-compatible API with thinking controls.", "GLM_API_KEY", mockProviderTemplate({ name: "glm-cn", kind: "openai", baseUrl: "https://open.bigmodel.cn/api/paas/v4", models: mockGLMAPIModels, visionModels: ["glm-5v-turbo"], default: "glm-5.2", apiKeyEnv: "GLM_API_KEY", contextWindow: 1000000, thinking: "enabled", supportedEfforts: ["enabled", "disabled"], defaultEffort: "enabled" })),
  mockPreset("zai-global", "Z.AI Global API", "Z.AI international OpenAI-compatible GLM API.", "ZAI_API_KEY", mockProviderTemplate({ name: "zai-global", kind: "openai", baseUrl: "https://api.z.ai/api/paas/v4", models: mockGLMAPIModels, visionModels: ["glm-5v-turbo"], default: "glm-5.2", apiKeyEnv: "ZAI_API_KEY", contextWindow: 1000000, thinking: "enabled", supportedEfforts: ["enabled", "disabled"], defaultEffort: "enabled" })),
  mockPreset("glm-coding-plan-cn", "GLM Coding Plan CN", "Zhipu GLM China coding-plan endpoint.", "GLM_PLAN_API_KEY", mockProviderTemplate({ name: "glm-coding-plan-cn", kind: "openai", baseUrl: "https://open.bigmodel.cn/api/coding/paas/v4", models: mockGLMCodingModels, default: "glm-5.2", apiKeyEnv: "GLM_PLAN_API_KEY", contextWindow: 1000000, thinking: "enabled", supportedEfforts: ["enabled", "disabled"], defaultEffort: "enabled" })),
  mockPreset("glm-coding-plan-cn-anthropic", "GLM Coding Plan CN Anthropic", "Zhipu GLM China coding-plan Anthropic-compatible endpoint.", "GLM_PLAN_API_KEY", mockProviderTemplate({ name: "glm-coding-plan-cn-anthropic", kind: "anthropic", baseUrl: "https://open.bigmodel.cn/api/anthropic", models: mockGLMAnthropicModels, default: "glm-5.2[1m]", apiKeyEnv: "GLM_PLAN_API_KEY", authHeader: true, thinking: "adaptive", contextWindow: 1000000 })),
  mockPreset("zai-coding-plan-global", "Z.AI Coding Plan Global", "Z.AI international coding-plan endpoint.", "ZAI_CODING_API_KEY", mockProviderTemplate({ name: "zai-coding-plan-global", kind: "openai", baseUrl: "https://api.z.ai/api/coding/paas/v4", models: mockGLMCodingModels, default: "glm-5.2", apiKeyEnv: "ZAI_CODING_API_KEY", contextWindow: 1000000, thinking: "enabled", supportedEfforts: ["enabled", "disabled"], defaultEffort: "enabled" })),
  mockPreset("zai-coding-plan-global-anthropic", "Z.AI Coding Plan Global Anthropic", "Z.AI international coding-plan Anthropic-compatible endpoint.", "ZAI_CODING_API_KEY", mockProviderTemplate({ name: "zai-coding-plan-global-anthropic", kind: "anthropic", baseUrl: "https://api.z.ai/api/anthropic", models: mockGLMAnthropicModels, default: "glm-5.2[1m]", apiKeyEnv: "ZAI_CODING_API_KEY", authHeader: true, thinking: "adaptive", contextWindow: 1000000 })),
  mockPreset("opencode-go", "OpenCode Go", "OpenCode Go relay with per-model capability overrides.", "OPENCODE_GO_API_KEY", mockProviderTemplate({ name: "opencode-go", kind: "openai", baseUrl: "https://opencode.ai/zen/go/v1", models: mockOpenCodeGoModels, default: "glm-5.2", apiKeyEnv: "OPENCODE_GO_API_KEY", contextWindow: 128000 })),
  mockPreset("opencode-go-anthropic", "OpenCode Go Anthropic", "OpenCode Go subscription Anthropic-compatible route for Qwen and MiniMax models.", "OPENCODE_GO_API_KEY", mockProviderTemplate({ name: "opencode-go-anthropic", kind: "anthropic", baseUrl: "https://opencode.ai/zen/go", models: mockOpenCodeGoAnthropicModels, visionModels: ["qwen3.7-plus", "qwen3.6-plus"], default: "qwen3.7-plus", apiKeyEnv: "OPENCODE_GO_API_KEY", thinking: "adaptive", contextWindow: 262144 })),
  mockPreset("opencode-zen-anthropic", "OpenCode Zen Anthropic", "OpenCode Zen Anthropic-compatible route for Claude and Qwen models.", "OPENCODE_API_KEY", mockProviderTemplate({ name: "opencode-zen-anthropic", kind: "anthropic", baseUrl: "https://opencode.ai/zen", models: mockOpenCodeZenAnthropicModels, visionModels: ["claude-sonnet-4-6", "claude-opus-4-8", "claude-haiku-4-5"], default: "claude-sonnet-4-6", apiKeyEnv: "OPENCODE_API_KEY", contextWindow: 262144 })),
  mockPreset("qwen-cn", "Qwen CN API", "Alibaba DashScope China standard OpenAI-compatible endpoint.", "QWEN_API_KEY", mockProviderTemplate({ name: "qwen-cn", kind: "openai", baseUrl: "https://dashscope.aliyuncs.com/compatible-mode/v1", models: mockQwenAPIModels, visionModels: ["qwen3.7-plus", "qwen3.6-plus", "qwen3.5-plus", "kimi-k2.5"], default: "qwen3.7-plus", apiKeyEnv: "QWEN_API_KEY" })),
  mockPreset("qwen-global", "Qwen Global API", "Alibaba DashScope international standard OpenAI-compatible endpoint.", "QWEN_API_KEY", mockProviderTemplate({ name: "qwen-global", kind: "openai", baseUrl: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1", models: mockQwenAPIModels, visionModels: ["qwen3.7-plus", "qwen3.6-plus", "qwen3.5-plus", "kimi-k2.5"], default: "qwen3.7-plus", apiKeyEnv: "QWEN_API_KEY" })),
  mockPreset("qwen-coding-plan-cn", "Qwen Coding Plan CN", "Alibaba Cloud Qwen Coding Plan China endpoint.", "QWEN_CODING_API_KEY", mockProviderTemplate({ name: "qwen-coding-plan-cn", kind: "openai", baseUrl: "https://coding.dashscope.aliyuncs.com/v1", models: mockQwenPlanModels, visionModels: mockQwenPlanVisionModels, default: "qwen3.7-plus", apiKeyEnv: "QWEN_CODING_API_KEY" })),
  mockPreset("qwen-coding-plan-cn-anthropic", "Qwen Coding Plan CN Anthropic", "Alibaba Cloud Qwen Coding Plan China Anthropic-compatible endpoint.", "QWEN_CODING_API_KEY", mockProviderTemplate({ name: "qwen-coding-plan-cn-anthropic", kind: "anthropic", baseUrl: "https://coding.dashscope.aliyuncs.com/apps/anthropic", models: mockQwenPlanModels, visionModels: mockQwenPlanVisionModels, default: "qwen3.7-plus", apiKeyEnv: "QWEN_CODING_API_KEY", thinking: "adaptive" })),
  mockPreset("qwen-coding-plan-global", "Qwen Coding Plan Global", "Alibaba Cloud Qwen Coding Plan international endpoint.", "QWEN_CODING_API_KEY", mockProviderTemplate({ name: "qwen-coding-plan-global", kind: "openai", baseUrl: "https://coding-intl.dashscope.aliyuncs.com/v1", models: mockQwenPlanModels, visionModels: mockQwenPlanVisionModels, default: "qwen3.7-plus", apiKeyEnv: "QWEN_CODING_API_KEY" })),
  mockPreset("qwen-coding-plan-global-anthropic", "Qwen Coding Plan Global Anthropic", "Alibaba Cloud Qwen Coding Plan international Anthropic-compatible endpoint.", "QWEN_CODING_API_KEY", mockProviderTemplate({ name: "qwen-coding-plan-global-anthropic", kind: "anthropic", baseUrl: "https://coding-intl.dashscope.aliyuncs.com/apps/anthropic", models: mockQwenPlanModels, visionModels: mockQwenPlanVisionModels, default: "qwen3.7-plus", apiKeyEnv: "QWEN_CODING_API_KEY", thinking: "adaptive" })),
  mockPreset("stepfun", "StepFun", "StepFun coding-plan OpenAI-compatible endpoint.", "STEPFUN_API_KEY", mockProviderTemplate({ name: "stepfun", kind: "openai", baseUrl: "https://api.stepfun.com/step_plan/v1", models: mockStepFunModels, default: "step-3.7-flash", apiKeyEnv: "STEPFUN_API_KEY", supportedEfforts: ["low", "medium", "high"], defaultEffort: "medium" })),
  mockPreset("stepfun-anthropic", "StepFun Anthropic", "StepFun coding-plan Anthropic-compatible endpoint.", "STEPFUN_API_KEY", mockProviderTemplate({ name: "stepfun-anthropic", kind: "anthropic", baseUrl: "https://api.stepfun.com/step_plan", models: mockStepFunModels, default: "step-3.7-flash", apiKeyEnv: "STEPFUN_API_KEY", thinking: "adaptive", supportedEfforts: ["low", "medium", "high"], defaultEffort: "medium" })),
  mockPreset("novita", "NovitaAI", "NovitaAI OpenAI-compatible multi-model gateway.", "NOVITA_API_KEY", mockProviderTemplate({ name: "novita", kind: "openai", baseUrl: "https://api.novita.ai/openai/v1", models: mockNovitaModels, default: "zai-org/glm-5.2", apiKeyEnv: "NOVITA_API_KEY" })),
  mockPreset("gmi", "GMI Cloud", "GMI Cloud direct multi-model OpenAI-compatible gateway.", "GMI_API_KEY", mockProviderTemplate({ name: "gmi", kind: "openai", baseUrl: "https://api.gmi-serving.com/v1", models: mockGMIModels, default: "zai-org/GLM-5.2-FP8", apiKeyEnv: "GMI_API_KEY", headers: { "User-Agent": "Reames Agent" } })),
  mockPreset("vercel-ai-gateway", "Vercel AI Gateway", "Vercel AI Gateway via Anthropic-compatible Messages API.", "AI_GATEWAY_API_KEY", mockProviderTemplate({ name: "vercel-ai-gateway", kind: "anthropic", baseUrl: "https://ai-gateway.vercel.sh", models: mockVercelModels, visionModels: ["anthropic/claude-sonnet-4.6", "anthropic/claude-opus-4.8", "openai/gpt-5.4", "openai/gpt-5.4-pro", "moonshotai/kimi-k2.7-code"], default: "anthropic/claude-sonnet-4.6", apiKeyEnv: "AI_GATEWAY_API_KEY", authHeader: true, contextWindow: 1000000 })),
  mockPreset("huggingface", "HuggingFace Router", "HuggingFace Inference Router OpenAI-compatible endpoint.", "HF_TOKEN", mockProviderTemplate({ name: "huggingface", kind: "openai", baseUrl: "https://router.huggingface.co/v1", models: ["zai-org/GLM-5.2", "deepseek-ai/DeepSeek-V3.2", "Qwen/Qwen3.5-72B-Instruct"], default: "zai-org/GLM-5.2", apiKeyEnv: "HF_TOKEN" })),
  mockPreset("nvidia", "NVIDIA NIM", "NVIDIA NIM OpenAI-compatible accelerated inference endpoint.", "NVIDIA_API_KEY", mockProviderTemplate({ name: "nvidia", kind: "openai", baseUrl: "https://integrate.api.nvidia.com/v1", models: ["nvidia/nemotron-3-nano-30b-a3b", "nvidia/nemotron-3-super-120b-a12b", "nvidia/nemotron-3-ultra-550b-a55b", "deepseek-ai/deepseek-v4-pro", "qwen/qwen3.5-397b-a17b"], default: "nvidia/nemotron-3-nano-30b-a3b", apiKeyEnv: "NVIDIA_API_KEY" })),
  mockPreset("kilocode", "KiloCode", "Kilo Code gateway OpenAI-compatible endpoint.", "KILOCODE_API_KEY", mockProviderTemplate({ name: "kilocode", kind: "openai", baseUrl: "https://api.kilo.ai/api/gateway", models: ["kilo/auto"], default: "kilo/auto", apiKeyEnv: "KILOCODE_API_KEY" })),
  mockPreset("ollama-cloud", "Ollama Cloud", "Hosted Ollama Cloud OpenAI-compatible endpoint with max reasoning effort.", "OLLAMA_API_KEY", mockProviderTemplate({ name: "ollama-cloud", kind: "openai", baseUrl: "https://ollama.com/v1", models: mockOllamaCloudModels, default: "glm-5.2", apiKeyEnv: "OLLAMA_API_KEY" })),
];

function mockProviderPresetViews(): ProviderPresetView[] {
  return [...mockProviderPresetTemplates].sort((a, b) => mockProviderPresetDisplayRank(a.id) - mockProviderPresetDisplayRank(b.id)).map((template) => ({
    id: template.id,
    label: template.label,
    description: template.description,
    keyEnv: template.keyEnv,
    providerNames: [template.provider.name],
    models: [...template.provider.models],
    added: false,
    status: "available",
    statusProviderNames: [],
    keySet: false,
    requiresKey: true,
    configured: false,
  }));
}

function mockProviderPresetDisplayRank(id: string): number {
  if (id === "glm-cn" || id === "zai-global" || id.startsWith("glm-coding-plan-") || id.startsWith("zai-coding-plan-")) return 0;
  if (id.startsWith("longcat-")) return 1;
  if (id.startsWith("kimi-")) return 2;
  if (id.startsWith("minimax-")) return 3;
  return 4;
}

function cloneMockProviderTemplate(id: string, key: string): ProviderView | undefined {
  const template = mockProviderPresetTemplates.find((candidate) => candidate.id === id);
  if (!template) return undefined;
  return {
    ...JSON.parse(JSON.stringify(template.provider)) as ProviderView,
    keySet: Boolean(key.trim()),
  };
}

const mockPreviewImageDataURL =
  "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='160' height='120' viewBox='0 0 160 120'%3E%3Cdefs%3E%3ClinearGradient id='g' x1='0' y1='0' x2='1' y2='1'%3E%3Cstop offset='0' stop-color='%23f97316'/%3E%3Cstop offset='1' stop-color='%232563eb'/%3E%3C/linearGradient%3E%3C/defs%3E%3Crect width='160' height='120' rx='14' fill='url(%23g)'/%3E%3Ccircle cx='44' cy='38' r='16' fill='%23fff7ed' opacity='.9'/%3E%3Cpath d='M18 96 62 58l24 22 18-16 38 32z' fill='%23ffffff' opacity='.9'/%3E%3C/svg%3E";

export function makeMockApp(): AppBindings {
  const scenario = mockScenario();
  const freshMock = scenario === "fresh";
  const guidanceMock = scenario === "guidance";
  const runningMock = scenario === "running" || guidanceMock;
  const sandboxEscapeMock = scenario === "sandbox_escape";
  const mockAttachmentDataURLs = new Map<string, string>();
  let cancelled = false;
  let pendingAskPreview = false;
  let pendingApprovalPreview = false;
  let onboardingDismissed = false;
  const globalWorkspaceRoot = "~/Library/Application Support/reames-agent/global-workspace";
  let cwd = freshMock ? globalWorkspaceRoot : "~/projects/joyquant-db"; // mutable so PickWorkspace is visible in dev
  let workspaces = freshMock ? [] : ["~/projects/joyquant-db", "~/projects/joyquant-sys", "~/projects/reames-agent", "~/projects/blade"];
  let mockEffort = "auto";
  let mockDesktopZoomFactor = 1.0;
  const day = 86_400_000;
  const t0 = Date.now();
  // Mutable so MCP add/remove/retry are observable in browser dev.
  let capServers: ServerView[] = [
    {
      name: "github",
      transport: "stdio",
      status: "connected",
      configured: true,
      autoStart: true,
      tier: "background",
      command: "npx",
      args: ["-y", "@modelcontextprotocol/server-github"],
      tools: 4,
      prompts: 2,
      resources: 0,
      trustedReadOnlyTools: ["pull_request_read"],
      toolList: [
        { name: "issue_read", description: "Read GitHub issue details and comments.", readOnlyHint: true },
        { name: "pull_request_read", description: "Read pull request metadata, files, and review threads.", readOnlyHint: true },
        { name: "search_issues", description: "Search issues and pull requests.", readOnlyHint: true },
        { name: "issue_write", description: "Create or update GitHub issues." },
      ],
    },
    {
      name: "linear",
      transport: "http",
      status: "initializing",
      configured: true,
      autoStart: true,
      tier: "background",
      url: "https://mcp.linear.app/mcp",
      authStatus: "possible",
      authUrl: "https://mcp.linear.app/mcp",
      tools: 8,
      prompts: 0,
      resources: 0,
      toolList: [
        { name: "list_issues", description: "List and filter Linear issues." },
        { name: "get_issue", description: "Fetch a Linear issue by id or key." },
        { name: "create_issue", description: "Create a Linear issue." },
        { name: "update_issue", description: "Update status, assignee, priority, or labels." },
        { name: "list_projects", description: "List Linear projects." },
        { name: "get_project", description: "Fetch project details." },
        { name: "list_teams", description: "List Linear teams." },
        { name: "search", description: "Search Linear workspace objects." },
      ],
    },
    { name: "figma", transport: "http", status: "failed", configured: true, autoStart: true, tier: "background", url: "https://mcp.figma.com/mcp", authStatus: "required", authUrl: "https://mcp.figma.com/mcp", tools: 0, prompts: 0, resources: 0, error: "connect: 401 unauthorized" },
  ];
  const capSkills: SkillView[] = [
    { name: "explore", description: "Investigate the codebase in an isolated subagent", scope: "builtin", runAs: "subagent", enabled: true },
    { name: "review", description: "Review the staged diff", scope: "project", runAs: "inline", enabled: false },
    { name: "init", description: "Scaffold a REASONIX.md for this repo", scope: "builtin", runAs: "inline", enabled: true },
  ];
  let capSkillRoots: SkillRootView[] = [
    { dir: "~/projects/reames-agent/.reames-agent/skills", scope: "project", priority: 1, status: "missing", configured: false, removable: true, skills: 0 },
    {
      dir: "~/my-skills",
      scope: "custom",
      priority: 5,
      status: "ok",
      configured: true,
      removable: true,
      skills: 1,
      skillItems: [{ name: "review", description: "Review the staged diff", scope: "custom", runAs: "inline" }],
    },
    {
      dir: "~/.reames-agent/skills",
      scope: "global",
      priority: 6,
      status: "ok",
      configured: false,
      removable: true,
      skills: 2,
      skillItems: [
        { name: "explore", description: "Investigate the codebase in an isolated subagent", scope: "global", runAs: "subagent" },
        { name: "init", description: "Scaffold a REASONIX.md for this repo", scope: "global", runAs: "inline" },
      ],
    },
  ];
  let capPlugins: PluginView[] = [];
  const mockPluginPlan = (op: string, name: string, action: string, source = ""): PluginOperationView => ({
    ok: true,
    status: "planned",
    op,
    applied: false,
    source: source || undefined,
    name,
    kind: "plugin",
    kinds: { skill: 0, mcp: 0, plugin: 1 },
    scope: "global",
    mode: "copy",
    planId: `mock:${op}:${name}:${source}`,
    actions: [{
      kind: "plugin",
      action,
      name,
      source: source || undefined,
      status: "planned",
      riskLevel: op === "install" ? "medium" : "high",
      permissions: ["skills.load"],
      trustStatus: source.startsWith("git:") ? "github-https-unsigned" : "local-snapshot",
      rollbackAvailable: op === "rollback",
    }],
  });
  const mockPluginDone = (plan: PluginOperationView): PluginOperationView => ({
    ...plan,
    ok: true,
    status: "done",
    applied: true,
    actions: plan.actions.map((action) => ({ ...action, status: "done" })),
  });
  const mockSwitchWorkspace = async (path: string) => {
    cwd = path || "~";
    workspaces = [cwd, ...workspaces.filter((p) => p !== cwd)].slice(0, 12);
    if (!mockProjectTree.some((node) => node.kind === "project" && node.root === cwd)) {
      mockProjectTree.unshift({
        key: `project_${cwd}`,
        kind: "project",
        label: baseName(cwd),
        root: cwd,
        children: [],
      });
    }
    return cwd;
  };
  // Mutable so delete/rename are observable in browser dev.
  const sessions: SessionMeta[] = [
    { path: "/mock/sessions/a.jsonl", preview: "fix the login bug in auth.go", turns: 12, createdAt: t0 - 2 * day, lastActivityAt: t0 - 3_600_000, modTime: t0 - 3_600_000, current: true, open: true },
    { path: "/mock/sessions/b.jsonl", preview: "refactor the payment module", turns: 5, createdAt: t0 - 3 * day, lastActivityAt: t0 - 6 * 3_600_000, modTime: t0 - 6 * 3_600_000, current: false, open: true },
    { path: "/mock/sessions/c.jsonl", preview: "write the README and badges", turns: 8, createdAt: t0 - 4 * day, lastActivityAt: t0 - day - 3_600_000, modTime: t0 - day - 3_600_000, current: false, open: false },
    { path: "/mock/sessions/d.jsonl", preview: "explain the plugin host design", turns: 3, createdAt: t0 - 5 * day, lastActivityAt: t0 - 4 * day, modTime: t0 - 4 * day, current: false, open: false },
  ];
  const trashedSessions: SessionMeta[] = [
    {
      path: "/mock/sessions/.trash/trash-dev-standard.jsonl",
      title: t("mock.trashDevStandardTitle"),
      preview: t("mock.trashDevStandardPreview"),
      turns: 4,
      createdAt: t0 - 8 * day,
      lastActivityAt: t0 - 7 * day,
      modTime: t0 - 7 * day,
      deletedAt: t0 - 20 * 60_000,
      current: false,
      open: false,
      scope: "project",
      workspaceRoot: "~/projects/joyquant-db",
      topicId: "topic_dev_standard",
      topicTitle: t("mock.trashDevStandardTitle"),
    },
    {
      path: "/mock/sessions/.trash/trash-p3a-review.jsonl",
      title: t("mock.trashP3aTitle"),
      preview: t("mock.trashP3aPreview"),
      turns: 7,
      createdAt: t0 - 6 * day,
      lastActivityAt: t0 - 5 * day,
      modTime: t0 - 5 * day,
      deletedAt: t0 - 2 * 3_600_000,
      current: false,
      open: false,
      scope: "project",
      workspaceRoot: "~/projects/joyquant-sys",
      topicId: "topic_p3a_pd",
      topicTitle: t("mock.trashP3aTitle"),
    },
    {
      path: "/mock/sessions/.trash/trash-global-product.jsonl",
      title: t("mock.trashGlobalProductTitle"),
      preview: t("mock.trashGlobalProductPreview"),
      turns: 2,
      createdAt: t0 - 4 * day,
      lastActivityAt: t0 - 3 * day,
      modTime: t0 - 3 * day,
      deletedAt: t0 - day,
      current: false,
      open: false,
      scope: "global",
      topicId: "topic_product",
      topicTitle: t("mock.trashGlobalProductTitle"),
    },
  ];
  if (freshMock) {
    sessions.splice(0);
    trashedSessions.splice(0);
  }
  // Mutable settings so the Settings panel's edits are observable in browser dev.
  const settings: SettingsView = {
    defaultModel: "deepseek",
    plannerModel: "",
    subagentModel: "",
    subagentEffort: "",
    autoPlan: "off",
    providers: [
      { name: "deepseek", builtIn: true, added: false, kind: "openai", baseUrl: "https://api.deepseek.com", modelsUrl: "", models: ["deepseek-v4-flash"], visionModels: [], visionModelsConfigured: false, default: "deepseek-v4-flash", apiKeyEnv: "DEEPSEEK_API_KEY", keySet: true, balanceUrl: "https://api.deepseek.com/user/balance", contextWindow: 1_000_000, reasoningProtocol: "", thinking: "", supportedEfforts: [], defaultEffort: "" },
    ],
    officialProviders: [
      { name: "deepseek", builtIn: true, added: false, kind: "openai", baseUrl: "https://api.deepseek.com", modelsUrl: "", models: ["deepseek-v4-flash", "deepseek-v4-pro"], visionModels: [], visionModelsConfigured: false, default: "deepseek-v4-flash", apiKeyEnv: "DEEPSEEK_API_KEY", keySet: true, balanceUrl: "https://api.deepseek.com/user/balance", contextWindow: 1_000_000, reasoningProtocol: "", thinking: "", supportedEfforts: [], defaultEffort: "" },
    ],
    providerPresets: mockProviderPresetViews(),
    permissions: { mode: "ask", allow: ["ls", "read_file"], ask: [], deny: ["Bash(rm:*)"] },
    sandbox: { bash: "enforce", network: true, workspaceRoot: "", allowWrite: [], effectiveWorkspaceRoot: cwd, effectiveWriteRoots: [cwd], shell: "auto" },
    network: {
      proxyMode: "auto",
      proxyUrl: "",
      noProxy: "",
      proxy: { type: "socks5", server: "127.0.0.1", port: 7890, username: "", password: "" },
    },
    agent: { temperature: 0.2, maxSteps: 0, plannerMaxSteps: 0, maxSubagentDepth: 2, systemPrompt: "You are Reames Agent, a coding agent.", coldResumePrune: true, reasoningLanguage: "auto" },
    bot: {
      enabled: !freshMock,
      model: "",
      toolApprovalMode: "ask",
      maxSteps: 25,
      debounceMs: 1500,
      queueMode: "steer",
      queueCap: 20,
      queueDrop: "summarize",
      ignoreSelfMessages: true,
      selfUserIds: {
        qq: [],
        feishu: [],
        weixin: [],
      },
      control: {
        enabled: false,
        addr: "127.0.0.1:37913",
        tokenEnv: "REAMES_AGENT_BOT_CONTROL_TOKEN",
      },
      pairing: {
        enabled: true,
        requestTtlMinutes: 60,
        maxPendingPerPlatform: 3,
      },
      routes: [],
      allowlist: {
        enabled: true,
        allowAll: false,
        qqUsers: [],
        feishuUsers: freshMock ? [] : ["ou_mock_user_001"],
        weixinUsers: freshMock ? [] : ["wxid_mock_user_001"],
        qqApprovers: [],
        feishuApprovers: [],
        weixinApprovers: [],
        qqAdmins: [],
        feishuAdmins: [],
        weixinAdmins: [],
        qqGroups: [],
        feishuGroups: [],
        weixinGroups: [],
      },
      qq: { enabled: false, appId: "", appSecretEnv: "QQ_BOT_APP_SECRET", secretSet: false, sandbox: false, model: "", toolApprovalMode: "ask", workspaceRoot: "", access: { enabled: true, allowAll: false, pairingEnabled: true, users: [], groups: [], approvers: [], admins: [] } },
      feishu: {
        enabled: false,
        domain: "feishu",
        appId: "",
        appSecretEnv: "FEISHU_BOT_APP_SECRET",
        secretSet: false,
        verificationToken: "",
        mode: "webhook",
        webhookPort: 8080,
        requireMention: true,
      },
      weixin: {
        enabled: false,
        accountId: "default",
        tokenEnv: "WEIXIN_BOT_TOKEN",
        tokenSet: false,
        apiBase: "https://ilinkai.weixin.qq.com",
      },
      connections: freshMock ? [] : [
        {
          id: "mock-lark-kun",
          provider: "feishu",
          domain: "lark",
          label: "kun",
          enabled: true,
          status: "connected",
	          model: "",
	          toolApprovalMode: "",
	          workspaceRoot: "",
	          access: { enabled: true, allowAll: false, pairingEnabled: true, users: ["ou_mock_user_001"], groups: [], approvers: [], admins: [] },
	          credential: {
            appId: "cli_mock_lark",
            appSecretEnv: "FEISHU_BOT_APP_SECRET",
            accountId: "",
            tokenEnv: "",
            secretSet: true,
          },
          sessionMappings: [
            {
              remoteId: "ou_mock_user_001",
              sessionId: "topic:topic_product",
              sessionSource: "",
              chatType: "",
              userId: "",
              threadId: "",
              scope: "global",
              workspaceRoot: "",
              updatedAt: new Date(Date.now() - 4 * 60_000).toISOString(),
            },
          ],
          lastError: "",
          createdAt: new Date(Date.now() - 86_400_000).toISOString(),
          updatedAt: new Date(Date.now() - 4 * 60_000).toISOString(),
        },
        {
          id: "mock-weixin-kun",
          provider: "weixin",
          domain: "weixin",
          label: "kun",
          enabled: true,
          status: "connected",
	          model: "",
	          toolApprovalMode: "",
	          workspaceRoot: "",
	          access: { enabled: true, allowAll: false, pairingEnabled: true, users: ["wxid_mock_user_001"], groups: [], approvers: [], admins: [] },
	          credential: {
            appId: "",
            appSecretEnv: "",
            accountId: "default",
            tokenEnv: "WEIXIN_BOT_TOKEN",
            secretSet: true,
          },
          sessionMappings: [
            {
              remoteId: "wxid_mock_user_001",
              sessionId: "topic:topic_ai",
              sessionSource: "",
              chatType: "",
              userId: "",
              threadId: "",
              scope: "global",
              workspaceRoot: "",
              updatedAt: new Date(Date.now() - 12 * 60_000).toISOString(),
            },
          ],
          lastError: "",
          createdAt: new Date(Date.now() - 86_400_000).toISOString(),
          updatedAt: new Date(Date.now() - 12 * 60_000).toISOString(),
        },
      ],
    },
    desktopLanguage: "",
    desktopLayoutStyle: "workbench",
    desktopTheme: "auto",
    desktopThemeStyle: "graphite",
    closeBehavior: "background",
    displayMode: "compact",
    statusBarStyle: "text",
    statusBarItems: [...DEFAULT_STATUS_BAR_ITEMS],
    defaultToolApprovalMode: "ask",
    checkUpdates: true,
    telemetry: true,
    metrics: true,
    memoryCompilerEnabled: true,
    configPath: "~/projects/reames-agent/reames-agent.toml",
    providerKinds: ["openai", "anthropic"],
    autoApproveTools: false,
    bypass: false,
  };
  const hookEvents = ["PreToolUse", "PostToolUse", "UserPromptSubmit", "Stop", "PostLLMCall", "SessionStart", "SessionEnd", "SubagentStop", "Notification", "PreCompact"];
  const hookSettings: Record<string, HooksSettingsView> = {
    global: {
      scope: "global",
      path: "~/.reames-agent/settings.json",
      projectRoot: "",
      trusted: true,
      events: hookEvents,
      hooks: [
        { event: "Stop", command: "echo turn done", description: "Notify after each turn" },
      ],
    },
    project: {
      scope: "project",
      path: "./.reames-agent/settings.json",
      projectRoot: "/mock/project",
      trusted: false,
      events: hookEvents,
      hooks: [],
    },
  };
  settings.providers = settings.providers.map((provider) =>
    provider.apiKeyEnv === "DEEPSEEK_API_KEY" ? { ...provider, keySet: !freshMock } : provider,
  );
  if (freshMock) {
    settings.configPath = "~/.config/reames-agent/config.toml";
  }
  const mockNow = Date.now();
  const mockProjectTree: ProjectNode[] = freshMock ? [] : [
    {
      key: "project_~/projects/joyquant-db",
      kind: "project",
      label: t("mock.projectJoyquantDb"),
      root: "~/projects/joyquant-db",
      projectColor: "blue",
      children: [
        { key: "topic_dev_standard", kind: "topic", label: `● ${t("mock.topicDevStandard")}`, root: "~/projects/joyquant-db", topicId: "topic_dev_standard", projectColor: "blue", turns: 18, lastActivityAt: mockNow - 8 * 60_000, open: true, running: runningMock },
        { key: "topic_db_maint", kind: "topic", label: t("mock.topicDbMaint"), root: "~/projects/joyquant-db", topicId: "topic_db_maint", projectColor: "blue", turns: 7, lastActivityAt: mockNow - 2 * 60 * 60_000 },
        { key: "topic_env", kind: "topic", label: t("mock.topicEnv"), root: "~/projects/joyquant-db", topicId: "topic_env", projectColor: "blue", turns: 3, lastActivityAt: mockNow - 26 * 60 * 60_000 },
      ],
    },
    {
      key: "project_~/projects/joyquant-sys",
      kind: "project",
      label: t("mock.projectJoyquantSys"),
      root: "~/projects/joyquant-sys",
      projectColor: "purple",
      children: [
        { key: "topic_p3b_pd", kind: "topic", label: `● ${t("mock.topicP3b")}`, root: "~/projects/joyquant-sys", topicId: "topic_p3b_pd", projectColor: "purple", turns: 11, lastActivityAt: mockNow - 3 * 24 * 60 * 60_000, status: runningMock ? "streaming" : undefined },
        { key: "topic_p3a_pd", kind: "topic", label: t("mock.topicP3a"), root: "~/projects/joyquant-sys", topicId: "topic_p3a_pd", projectColor: "purple", turns: 9, lastActivityAt: mockNow - 4 * 24 * 60 * 60_000, status: runningMock ? "thinking" : undefined },
        { key: "topic_hotfix", kind: "topic", label: t("mock.topicHotfix"), root: "~/projects/joyquant-sys", topicId: "topic_hotfix", projectColor: "purple", turns: 4, lastActivityAt: mockNow - 5 * 24 * 60 * 60_000, status: runningMock ? "thinking" : undefined },
        { key: "topic_sys_coord", kind: "topic", label: t("mock.topicSysCoord"), root: "~/projects/joyquant-sys", topicId: "topic_sys_coord", projectColor: "purple", turns: 14, lastActivityAt: mockNow - 6 * 24 * 60 * 60_000, status: runningMock ? "waiting_confirmation" : undefined },
        { key: "topic_sys_standard", kind: "topic", label: t("mock.topicSysStandard"), root: "~/projects/joyquant-sys", topicId: "topic_sys_standard", projectColor: "purple", turns: 6, lastActivityAt: mockNow - 7 * 24 * 60 * 60_000, status: "paused" },
        { key: "topic_sys_exception", kind: "topic", label: t("mock.topicSysException"), root: "~/projects/joyquant-sys", topicId: "topic_sys_exception", projectColor: "purple", turns: 2, lastActivityAt: mockNow - 8 * 24 * 60 * 60_000, status: "error" },
      ],
    },
    {
      key: "global_folder",
      kind: "global_folder",
      label: "Global",
      root: globalWorkspaceRoot,
      children: [
        { key: "global_topic_product", kind: "global_topic", label: t("mock.topicProduct"), topicId: "topic_product", turns: 5, lastActivityAt: mockNow - 8 * 24 * 60 * 60_000 },
        { key: "global_topic_ai", kind: "global_topic", label: t("mock.topicAi"), topicId: "topic_ai", turns: 8, lastActivityAt: mockNow - 10 * 24 * 60 * 60_000 },
        { key: "global_topic_lab", kind: "global_topic", label: t("mock.topicLab"), topicId: "topic_lab", turns: 2, lastActivityAt: mockNow - 12 * 24 * 60 * 60_000 },
      ],
    },
  ];
  const ensureMockGlobalFolder = (): ProjectNode => {
    let node = mockProjectTree.find((item) => item.kind === "global_folder");
    if (!node) {
      node = {
        key: "global_folder",
        kind: "global_folder",
        label: "Global",
        root: globalWorkspaceRoot,
        children: [],
      };
      mockProjectTree.push(node);
    }
    return node;
  };
  const mockProjectTreeForDisplay = () => {
    const pinnedProjects = mockProjectTree.filter((node) => node.kind === "project" && node.pinned);
    if (pinnedProjects.length === 0) return mockProjectTree;
    const rest = mockProjectTree.filter((node) => !(node.kind === "project" && node.pinned));
    return [...pinnedProjects, ...rest];
  };
  const cloneProjectTree = () => {
    if (mockProjectTree.length === 0) ensureMockGlobalFolder();
    return JSON.parse(JSON.stringify(mockProjectTreeForDisplay())) as ProjectNode[];
  };
  const projectChildren = (node: ProjectNode): ProjectNode[] => Array.isArray(node.children) ? node.children : [];
  const findMockTopic = (topicId: string): ProjectNode | null => {
    for (const parent of mockProjectTree) {
      const found = projectChildren(parent).find((child) => child.topicId === topicId);
      if (found) return found;
    }
    return null;
  };
  const setMockTopicPinned = (topicId: string, pinned: boolean) => {
    for (const parent of mockProjectTree) {
      const children = projectChildren(parent);
      const index = children.findIndex((child) => child.topicId === topicId);
      if (index < 0) continue;
      const topic = { ...children[index], pinned: pinned || undefined };
      if (!pinned) {
        parent.children = children.map((child, i) => (i === index ? topic : child));
        return;
      }
      const remaining = children.filter((_, i) => i !== index);
      parent.children = [topic, ...remaining];
      return;
    }
  };
  const setMockProjectPinned = (workspaceRoot: string, pinned: boolean) => {
    const index = mockProjectTree.findIndex((node) => node.kind === "project" && node.root === workspaceRoot);
    if (index < 0) return;
    mockProjectTree[index] = { ...mockProjectTree[index], pinned: pinned || undefined };
  };
  const deleteMockTopic = (topicId: string) => {
    for (const parent of mockProjectTree) {
      parent.children = projectChildren(parent).filter((child) => child.topicId !== topicId);
    }
  };
  const topicLabel = (topicId: string, fallback: string) => (findMockTopic(topicId)?.label || fallback).replace(/^●\s*/, "");
  const mockTopicStatus = (topicId: string) => findMockTopic(topicId)?.status ?? "";
  const mockTopicIsRunning = (topicId: string) => {
    const status = mockTopicStatus(topicId);
    return status === "streaming" || status === "thinking" || status === "waiting_confirmation";
  };
  const mockTopicIsBlank = (topicId: string) => {
    const topic = findMockTopic(topicId);
    return Boolean(topic && topic.label === t("mock.newSession") && !topic.turns && !topic.lastActivityAt && !topic.status);
  };
  const mockTopicRunsInScenario = (topicId: string) => runningMock && mockTopicIsRunning(topicId);
  const mockLongTranscriptHistory = (): HistoryMessage[] => {
    const out: HistoryMessage[] = [];
    for (let i = 1; i <= 18; i++) {
      out.push({
        role: "user",
        content: `第 ${i} 轮：检查聊天滚动定位，切换会话后应该自动停在最新消息底部。`,
      });
      if (i === 4) {
        out.push({ role: "phase", content: "复现切换会话后的滚动位置" });
      }
      if (i === 8) {
        const toolID = "mock-scroll-layout-check";
        out.push({
          role: "assistant",
          content: "我会先读取滚动容器尺寸，再确认是否存在动态高度变化导致的底部偏移。",
          reasoning: "旧实现只重置 stick 标志，没有主动等待布局稳定；AskCard、Approval、Todo 这类卡片可能在下一帧改变高度。",
          toolCalls: [{ id: toolID, name: "bash", arguments: JSON.stringify({ command: "npm run check:css && pnpm typecheck" }) }],
        });
        out.push({
          role: "tool",
          toolCallId: toolID,
          toolName: "bash",
          content: "CSS syntax check passed\nz-index token check passed\ntsc --noEmit passed\n",
        });
        continue;
      }
      if (i === 13) {
        out.push({ role: "notice", level: "info", content: "模拟提示：用户向上查看历史后，右下角应出现跳到底部按钮。" });
      }
      out.push({
        role: "assistant",
        content: [
          `第 ${i} 轮结果：当前滚动契约会在切换会话或 reveal 信号到达后执行强制贴底。`,
          "它会先立即设置 scrollTop 到 scrollHeight，再连续几个 animation frame 复查，避免动态内容把底部再次推走。",
          "如果用户主动向上滚动，普通 streaming 不会强行拉回；只有点击跳到底部按钮或显式切换会话才会重新贴底。",
        ].join("\n\n"),
      });
    }
    out.push({
      role: "compaction",
      content: "",
      trigger: "manual",
      messages: 36,
      summary: "Mock 长会话用于验证桌面端 Transcript 自动贴底、多帧布局修正和跳到底部按钮。",
      archive: "mock-scroll-preview",
    });
    out.push({
      role: "assistant",
      content: "最终状态：这条消息应该位于真实底部。向上滚动后，右下角会显示跳到底部按钮；点击按钮后应回到这里。",
    });
    return out;
  };
	  const mockTopicHistory = (topicId: string): HistoryMessage[] => {
	    switch (topicId) {
      case "topic_product":
        return [
          {
            role: "user",
            content: [
              "[[reames-agent-im]]",
              "provider=lark",
              "label=Feishu / Lark",
              "sender=ou_mock_user_001",
              "chat=p2p 会话",
              "[[/reames-agent-im]]",
              "你可以做什么",
            ].join("\n"),
          },
          {
            role: "assistant",
            content: "这是 Global 范围下的 IM 会话。我可以先处理不依赖项目文件的问答、计划和信息整理；需要进入项目时，再由桌面端显式绑定或迁移到项目话题。",
          },
        ];
      case "topic_ai":
        return [
          {
            role: "user",
            content: [
              "[[reames-agent-im]]",
              "provider=weixin",
              "label=微信",
              "sender=wxid_mock_user_001",
              "chat=单聊",
              "[[/reames-agent-im]]",
              "帮我整理一下今天要做的事",
            ].join("\n"),
          },
          {
            role: "assistant",
            content: "可以。我会先在 Global 范围里整理任务清单；如果某条任务需要读取项目文件，再切到你授权的项目话题处理。",
          },
        ];
      case "topic_dev_standard":
        return mockLongTranscriptHistory();
      case "topic_p3b_pd":
        return [
          { role: "user", content: "把 p3b P&D 的范围和风险重新整理成可执行计划。" },
          { role: "phase", content: "分析需求范围" },
        ];
      case "topic_p3a_pd":
        return [
          { role: "user", content: "复盘 p3a 的技术方案，先不要写文件，先说明你的判断。" },
        ];
      case "topic_hotfix":
        return [
          { role: "user", content: "检查 post-p3-hotfix 的回归风险，重点看最近的 shell 输出和 git 改动。" },
          { role: "assistant", content: "", reasoning: "我先定位最近一次 hotfix 的上下文，然后用只读命令检查状态；左侧保持“思考中”，工具细节在这里展开。" },
        ];
      case "topic_sys_coord":
        return [
          { role: "user", content: "准备执行 joyquant-sys 的同步脚本，但需要我确认后再运行。" },
          { role: "assistant", content: "", reasoning: "这个动作会运行脚本并可能刷新本地缓存，所以需要先等用户确认。" },
        ];
      case "topic_sys_standard":
        return [
          { role: "user", content: "继续制定 SYS 项目开发规范，先停在当前检查点。" },
          { role: "assistant", content: "已暂停在规范整理阶段。当前保留了目录约定、分支策略和待确认的发布检查项；继续时可以从这里恢复。" },
          { role: "notice", level: "info", content: "会话已暂停：未继续执行命令，等待用户恢复或切换任务。" },
        ];
      case "topic_sys_exception":
        return [
          { role: "user", content: "演练异常处理流程，看看失败时界面怎么提示。" },
          { role: "assistant", content: "我尝试校验恢复脚本时遇到异常，已停止继续执行。" },
          { role: "notice", level: "warn", content: "运行异常：恢复脚本缺少必要环境变量 JOYQUANT_SYS_TOKEN。请补齐配置后重试。" },
        ];
      default:
        return [];
	    }
	  };
	  const mockHistoryPage = (messages: HistoryMessage[], beforeTurn = 0, limit = 60): HistoryPage => {
	    const totalTurns = messages.reduce((count, message) => count + (message.role === "user" ? 1 : 0), 0);
	    const safeLimit = Math.max(1, Math.min(200, Math.floor(limit || 60)));
	    const endTurn = beforeTurn > 0 && beforeTurn <= totalTurns ? beforeTurn : totalTurns;
	    const startTurn = Math.max(0, endTurn - safeLimit);
	    let turn = -1;
	    const pageMessages = messages.filter((message) => {
	      if (message.role === "user") turn += 1;
	      if (turn < 0) return startTurn === 0;
	      return turn >= startTurn && turn < endTurn;
	    });
	    return { messages: pageMessages, startTurn, endTurn, totalTurns, hasOlder: startTurn > 0 };
	  };
	  const mockRuntimeInjected = new Set<string>();
  const queueMockTopicRuntime = (tab: TabMeta) => {
    if (!runningMock) return;
    const status = mockTopicStatus(tab.topicId);
    if (status !== "streaming" && status !== "thinking" && status !== "waiting_confirmation") return;
    const key = `${tab.id}:${tab.topicId}:${status}`;
    if (mockRuntimeInjected.has(key)) return;
    mockRuntimeInjected.add(key);
    window.setTimeout(() => {
      void withMockTabScope(tab.id, async () => {
        emitMockTurnStarted();
        await delay(120);
        if (tab.topicId === "topic_p3b_pd") {
          const text = "我会先把范围拆成三层：目标、依赖、风险。当前已经确认 p3b 的交付边界，接下来补充每个模块的验收口径...";
          for (const ch of text) {
            emit({ kind: "text", text: ch });
            await delay(5);
          }
          return;
        }
        if (tab.topicId === "topic_p3a_pd") {
          emit({ kind: "reasoning", text: "我正在对比 p3a 和 p3b 的差异：先看约束，再看变更风险，最后判断是否需要拆成独立任务。\n\n" });
          await delay(220);
          emit({ kind: "reasoning", text: "当前倾向：先保留 p3a 的兼容路径，不急于删除旧逻辑。" });
          return;
        }
        if (tab.topicId === "topic_hotfix") {
          const id = "mock-hotfix-shell";
          emit({ kind: "tool_dispatch", tool: { id, name: "bash", args: JSON.stringify({ command: "git status --short && npm test" }), readOnly: true } });
          await delay(180);
          emit({ kind: "tool_progress", tool: { id, name: "bash", readOnly: true, output: "$ git status --short\n M internal/sys/runner.go\n\n$ npm test\nrunning targeted regression tests...\n" } });
          return;
        }
        if (tab.topicId === "topic_sys_coord") {
          pendingApprovalPreview = true;
          emit({ kind: "reasoning", text: "我已经准备好执行同步脚本，但这个操作会影响本地 workspace，需要用户确认。" });
          await delay(160);
          emit({
            kind: "approval_request",
            approval: {
              id: "mock-sys-confirm",
              tool: "bash",
              subject: "npm run sync:joyquant-sys\n\n该命令会同步 SYS 项目配置并刷新本地缓存。",
            },
          });
        }
      });
    }, 180);
  };
  const setMockActiveTab = (tabId: string) => {
    mockTabs = mockTabs.map((tab) => ({ ...tab, active: tab.id === tabId }));
  };
  const currentMockTurnTabId = () => mockScopedTabId || mockTabs.find((tab) => tab.active)?.id;
  const setMockTabRunning = (tabId: string | undefined, running: boolean) => {
    if (!tabId) return;
    mockTabs = mockTabs.map((tab) => (tab.id === tabId ? { ...tab, running } : tab));
  };
  const emitMockTurnStarted = () => {
    setMockTabRunning(currentMockTurnTabId(), true);
    emit({ kind: "turn_started" });
  };
  const emitMockTurnDone = () => {
    setMockTabRunning(currentMockTurnTabId(), false);
    emit({ kind: "turn_done" });
  };
  let mockTabs: TabMeta[] = freshMock ? [
    {
      id: "tab_global",
      scope: "global",
      workspaceRoot: globalWorkspaceRoot,
      workspaceName: "Global",
      workspacePath: globalWorkspaceRoot,
      topicId: "",
      topicTitle: "Global",
      label: "DeepSeek-R1",
      ready: true,
      running: false,
      mode: "normal",
      collaborationMode: "normal",
      toolApprovalMode: "ask",
      tokenMode: "full",
      active: true,
      cwd: globalWorkspaceRoot,
    },
  ] : [
    {
      id: "tab_joyquant_db",
      scope: "project",
      workspaceRoot: "~/projects/joyquant-db",
      workspaceName: "joyquant-db",
      workspacePath: "~/projects/joyquant-db",
      gitBranch: "main",
      topicId: "topic_dev_standard",
      topicTitle: t("mock.trashDevStandardTitle"),
      projectColor: "blue",
      label: "DeepSeek-R1",
      ready: true,
      running: false,
      mode: "normal",
      collaborationMode: "normal",
      toolApprovalMode: "ask",
      tokenMode: "full",
      active: !guidanceMock,
      cwd: "~/projects/joyquant-db",
    },
    {
      id: "tab_joyquant_sys",
      scope: "project",
      workspaceRoot: "~/projects/joyquant-sys",
      workspaceName: "joyquant-sys",
      workspacePath: "~/projects/joyquant-sys",
      gitBranch: "feature/p3b",
      topicId: "topic_p3b_pd",
      topicTitle: "p3b P&D",
      projectColor: "purple",
      label: "DeepSeek-R1",
      ready: true,
      running: runningMock && mockTopicIsRunning("topic_p3b_pd"),
      mode: "normal",
      collaborationMode: "normal",
      toolApprovalMode: "ask",
      tokenMode: "full",
      active: guidanceMock,
      cwd: "~/projects/joyquant-sys",
    },
    {
      id: "tab_global",
      scope: "global",
      workspaceRoot: "",
      workspaceName: "Global",
      workspacePath: "~/projects/joyquant-db",
      topicId: "topic_global",
      topicTitle: "Global",
      label: "DeepSeek-R1",
      ready: true,
      running: false,
      mode: "normal",
      collaborationMode: "normal",
      toolApprovalMode: "ask",
      tokenMode: "full",
      active: false,
      cwd: "~/projects/joyquant-db",
    },
  ];
  if (sandboxEscapeMock) {
    window.setTimeout(() => {
      if (pendingApprovalPreview) return;
      pendingApprovalPreview = true;
      emitMockTurnStarted();
      emit({ kind: "reasoning", text: t("mock.sandboxEscapeReasoning") });
      emit({
        kind: "approval_request",
        approval: {
          id: "mock-sandbox-escape-preview",
          tool: "sandbox_escape",
          subject: t("mock.sandboxEscapeSubject"),
          reason: t("mock.sandboxEscapeReason"),
        },
      });
    }, 800);
  }
  const mockModelCatalog = [
    { ref: "deepseek/deepseek-v4-flash", provider: "deepseek", model: "deepseek-v4-flash" },
    { ref: "deepseek/deepseek-v4-pro", provider: "deepseek", model: "deepseek-v4-pro" },
  ];
  const defaultMockModelRef = mockModelCatalog[0].ref;
  const mockModelRef = (name: string): string => {
    const trimmed = name.trim();
    if (!trimmed || trimmed === "DeepSeek-R1") return defaultMockModelRef;
    const exact = mockModelCatalog.find((model) => model.ref === trimmed);
    if (exact) return exact.ref;
    const byModel = mockModelCatalog.find((model) => model.model === trimmed);
    return byModel?.ref ?? trimmed;
  };
  const mockModelLabel = (ref: string): string => mockModelCatalog.find((model) => model.ref === mockModelRef(ref))?.model ?? ref.split("/").pop() ?? ref;
  const mockTabModelRef = (tab?: TabMeta): string => mockModelRef(tab?.label ?? "");
  const setMockTabModel = (tabID: string | undefined, name: string) => {
    const ref = mockModelRef(name);
    const label = mockModelLabel(ref);
    let applied = false;
    mockTabs = mockTabs.map((tab) => {
      const match = tabID ? tab.id === tabID : tab.active;
      if (!match) return tab;
      applied = true;
      return { ...tab, label };
    });
    if (!applied && mockTabs.length > 0) {
      mockTabs = mockTabs.map((tab, index) => (index === 0 ? { ...tab, label } : tab));
    }
  };
  return {
    async MinimiseMainWindow() {
      console.info("mock MinimiseMainWindow");
    },
    async ToggleMaximiseMainWindow() {
      console.info("mock ToggleMaximiseMainWindow");
    },
    async IsMainWindowMaximised() {
      return false;
    },
    async CloseMainWindow() {
      console.info("mock CloseMainWindow");
    },
    async Platform() {
      const override = browserPlatformOverride();
      if (override) return override;
      // Mirror the OS the browser dev mock runs on.
      const ua = typeof navigator !== "undefined" ? navigator.userAgent : "";
      if (/Win/i.test(ua)) return "windows";
      if (/Mac/i.test(ua)) return "darwin";
      return "linux";
    },
        async Submit(input) {
          cancelled = false;
      emitMockTurnStarted();
      const trimmedInput = input.trim().toLowerCase();
      const goalMatch = /^\/goal(?:\s+([\s\S]*))?$/.exec(input.trim());
      if (goalMatch) {
        const arg = stripGoalResearchFlags((goalMatch[1] ?? "").trim());
        const lowered = arg.toLowerCase();
        const active = mockTabs.find((tab) => tab.active);
        if (!arg || lowered === "status") {
          emit({ kind: "notice", level: "info", text: active?.goal ? `goal: ${active.goal}` : "goal: none" });
          emitMockTurnDone();
          return;
        }
        if (["clear", "off", "stop", "done"].includes(lowered)) {
          mockTabs = mockTabs.map((tab) => (tab.active ? { ...tab, goal: "", goalStatus: "stopped", collaborationMode: "normal" } : tab));
          emit({ kind: "notice", level: "info", text: "goal cleared" });
          emitMockTurnDone();
          return;
        }
        mockTabs = mockTabs.map((tab) => (tab.active ? { ...tab, goal: arg, goalStatus: "running", collaborationMode: "goal" } : tab));
        emit({ kind: "notice", level: "info", text: `goal set: ${arg}` });
        await delay(350);
        if (cancelled) return;
        const reply = `Autonomous goal run started for: **${arg}**\n\nMock run completed.\n\n[goal:complete]`;
        emit({ kind: "message", text: reply });
        mockTabs = mockTabs.map((tab) => (tab.active ? { ...tab, goal: "", goalStatus: "complete", collaborationMode: "normal" } : tab));
        emit({ kind: "notice", level: "info", text: "goal complete" });
        emitMockTurnDone();
        return;
      }
      if (trimmedInput === "/approve-preview" || trimmedInput === "approve preview" || trimmedInput === "approve预览") {
        pendingApprovalPreview = true;
        await delay(250);
        if (cancelled) return;
        emit({
          kind: "approval_request",
          approval: {
            id: "mock-approval-preview",
            tool: "bash",
            subject: t("mock.approvalSubject"),
          },
        });
        return;
      }
      if (
        trimmedInput === "/sandbox-escape-preview" ||
        trimmedInput === "sandbox escape preview" ||
        trimmedInput === "sandbox_escape preview" ||
        trimmedInput === "sandbox escape预览"
      ) {
        pendingApprovalPreview = true;
        await delay(250);
        if (cancelled) return;
        emit({
          kind: "approval_request",
          approval: {
            id: "mock-sandbox-escape-preview",
            tool: "sandbox_escape",
            subject: t("mock.sandboxEscapeSubject"),
            reason: t("mock.sandboxEscapeReason"),
          },
        });
        return;
      }
      if (
        trimmedInput === "/plan-approve-preview" ||
        trimmedInput === "plan approve preview" ||
        trimmedInput === "plan approve预览"
      ) {
        pendingApprovalPreview = true;
        await delay(250);
        if (cancelled) return;
        emit({
          kind: "approval_request",
          approval: {
            id: "mock-plan-approval-preview",
            tool: "exit_plan_mode",
            subject: "",
          },
        });
        return;
      }
      if (trimmedInput === "/ask-preview" || trimmedInput === "ask preview" || trimmedInput === "ask预览") {
        pendingAskPreview = true;
        await delay(250);
        if (cancelled) return;
        emit({
          kind: "ask_request",
          ask: {
            id: "mock-ask-preview",
            questions: [
              {
                id: "q1",
                header: t("mock.askQ1Header"),
                prompt: t("mock.askQ1Prompt"),
                options: [
                  { label: t("mock.askQ1Opt1Label"), description: t("mock.askQ1Opt1Desc") },
                  { label: t("mock.askQ1Opt2Label"), description: t("mock.askQ1Opt2Desc") },
                  { label: t("mock.askQ1Opt3Label"), description: t("mock.askQ1Opt3Desc") },
                ],
              },
              {
                id: "q2",
                header: t("mock.askQ2Header"),
                prompt: t("mock.askQ2Prompt"),
                options: [
                  { label: t("mock.askQ2Opt1Label"), description: t("mock.askQ2Opt1Desc") },
                  { label: t("mock.askQ2Opt2Label"), description: t("mock.askQ2Opt2Desc") },
                  { label: t("mock.askQ2Opt3Label"), description: t("mock.askQ2Opt3Desc") },
                ],
              },
            ],
          },
        });
        return;
      }
      if (trimmedInput === "/todo-preview" || trimmedInput === "todo preview" || trimmedInput === "todo预览") {
        await delay(250);
        if (cancelled) return;
        emit({
          kind: "tool_dispatch",
          tool: {
            id: "mock-todo-preview",
            name: "todo_write",
            args: JSON.stringify({
              todos: [
                { content: t("mock.todo1"), status: "completed" },
                { content: t("mock.todo2"), activeForm: t("mock.todo2ActiveForm"), status: "in_progress" },
                { content: t("mock.todo3"), status: "pending" },
              ],
            }),
            readOnly: false,
          },
        });
        await delay(150);
        emit({
          kind: "tool_result",
          tool: {
            id: "mock-todo-preview",
            name: "todo_write",
            args: JSON.stringify({
              todos: [
                { content: t("mock.todo1"), status: "completed" },
                { content: t("mock.todo2"), activeForm: t("mock.todo2ActiveForm"), status: "in_progress" },
                { content: t("mock.todo3"), status: "pending" },
              ],
            }),
            output: "todo list updated",
            readOnly: false,
            durationMs: 150,
          },
        });
        emitMockTurnDone();
        return;
      }
      if (trimmedInput === "/process-preview" || trimmedInput === "process preview" || trimmedInput === "过程预览") {
        await delay(200);
        if (cancelled) return;
        emit({ kind: "phase", text: "Preparing context" });
        await delay(120);
        emit({ kind: "notice", level: "info", text: "Loaded project instructions from AGENTS.md." });
        await delay(120);
        emit({ kind: "notice", level: "warn", text: "Network access is enabled; external results may change over time." });
        await delay(120);
        emit({ kind: "compaction_started", compaction: { trigger: "manual" } });
        await delay(320);
        emit({
          kind: "compaction_done",
          compaction: {
            trigger: "manual",
            messages: 6,
            summary: "Preserved the active task, relevant files, and UI decisions while trimming earlier exploratory context.",
          },
        });
        emit({ kind: "message", text: "Process card preview complete." });
        emitMockTurnDone();
        return;
      }
      if (trimmedInput === "/nested-preview" || trimmedInput === "nested preview" || trimmedInput === "嵌套预览") {
        const parentId = "mock-nested-explore";
        await delay(180);
        if (cancelled) return;
        emit({
          kind: "reasoning",
          text: "我先快速探索相关文件，再整理这个工具行的视觉层级。",
        });
        emit({
          kind: "message",
          text: "",
          reasoning: "我先快速探索相关文件，再整理这个工具行的视觉层级。",
        });
        emit({
          kind: "tool_dispatch",
          tool: {
            id: parentId,
            name: "explore",
            args: JSON.stringify({ task: "在 Reames Agent 前端中检查工具调用图标和嵌套调用展示" }),
            readOnly: true,
            profile: { model: "mock-reames-agent", effort: "high" },
          },
        });
        for (let i = 1; i <= 30; i += 1) {
          if (cancelled) return;
          const id = `mock-nested-${i}`;
          const isSearch = i % 3 === 0;
          const name = isSearch ? "grep" : "read_file";
          const args = isSearch
            ? { pattern: i % 2 === 0 ? "tool__nested-count" : "explore", path: "desktop/frontend/src" }
            : { path: `desktop/frontend/src/${i % 2 === 0 ? "components/ToolCard.tsx" : "styles.css"}`, offset: i * 10, limit: 40 };
          emit({ kind: "tool_dispatch", tool: { id, name, args: JSON.stringify(args), readOnly: true, parentId } });
          emit({
            kind: "tool_result",
            tool: {
              id,
              name,
              readOnly: true,
              output: isSearch ? "3 matches" : "read 40 lines",
              durationMs: 24 + i,
            },
          });
          await delay(18);
        }
        emit({
          kind: "tool_result",
          tool: {
            id: parentId,
            name: "explore",
            readOnly: true,
            output: "已读 20 个文件 · 搜索 10 个文件",
            durationMs: 61510,
          },
        });
        emit({
          kind: "message",
          text: "Mock nested tool preview complete. The explore row now shows the compass count marker.",
        });
        emitMockTurnDone();
        return;
      }
      // Simulate the server's pre-first-token latency so the deferred user bubble
      // and the "un-send on Esc before any reply" path are observable in browser
      // dev. Bail if cancelled during the wait — nothing was streamed yet.
      await delay(700);
      if (cancelled) return;
      const reply =
        `You said: **${input}**\n\n` +
        "This is the browser dev mock — the real reply comes from the kernel " +
        "inside the Wails shell. Here's a fenced block to exercise the editor seam:\n\n" +
        "```go\nfunc main() {\n    println(\"hello from the mock\")\n}\n```\n";
      for (const ch of reply) {
        if (cancelled) break;
        emit({ kind: "text", text: ch });
        await delay(6);
      }
      emit({ kind: "message", text: reply });
      emit({
        kind: "tool_dispatch",
        tool: {
          id: "t1",
          name: "edit_file",
          args: '{"path":"main.go","old_string":"println(\\"hi\\")","new_string":"println(\\"hello\\")"}',
          readOnly: false,
        },
      });
      await delay(350);
      emit({
        kind: "tool_result",
        tool: { id: "t1", name: "edit_file", output: "edited main.go", readOnly: false, durationMs: 350 },
      });
      emit({
        kind: "usage",
        usage: {
          promptTokens: 1280,
          completionTokens: 64,
          totalTokens: 1344,
          cacheHitTokens: 1024,
          cacheMissTokens: 256,
          sessionCacheHitTokens: 1024,
          sessionCacheMissTokens: 256,
        },
      });
          emitMockTurnDone();
        },
        async SubmitToTab(_tabID, input) {
          await withMockTabScope(_tabID, () => this.Submit(input));
        },
        async SubmitDisplay(_display, input) {
          await this.Submit(input);
        },
        async SubmitDisplayToTab(_tabID, display, input) {
          await withMockTabScope(_tabID, () => this.SubmitDisplay(display, input));
        },
        async SubmitEditedDisplayToTab(_tabID, display, input, _original) {
          await withMockTabScope(_tabID, () => this.SubmitDisplay(display, input));
        },
        async RunShell(command) {
          cancelled = false;
          emitMockTurnStarted();
          await delay(100);
          if (cancelled) return;
          const id = `shell-${command.slice(0, 32)}`;
          emit({ kind: "tool_dispatch", tool: { id, name: "bash", args: JSON.stringify({ command }), readOnly: false } });
          await delay(200);
          if (cancelled) return;
          emit({ kind: "tool_progress", tool: { id, name: "bash", output: `$ ${command}\n(mock output)\n`, readOnly: false } });
          await delay(100);
          if (cancelled) return;
          emit({ kind: "tool_result", tool: { id, name: "bash", output: `$ ${command}\n(mock output)\n`, readOnly: false, durationMs: 300 } });
          emitMockTurnDone();
        },
        async RunShellForTab(_tabID, command) {
          await withMockTabScope(_tabID, () => this.RunShell(command));
        },
        async Steer(_text) {
          // Mock: emit a steer event as confirmation in the transcript.
          emit({ kind: "steer", text: _text });
        },
        async SteerForTab(_tabID, _text) {
          await this.Steer(_text);
        },
        async Cancel() {
          cancelled = true;
          emitMockTurnDone();
        },
        async CancelTab(_tabID) {
          await withMockTabScope(_tabID, () => this.Cancel());
        },
        async Approve(_id, allow, session, persist) {
          if (!pendingApprovalPreview) return;
          pendingApprovalPreview = false;
          const suffix = persist ? "grant saved" : session ? "grant active this session" : "allowed once";
          emit({
            kind: "message",
            text: `approval preview answered: ${allow ? suffix : "denied"}`,
          });
          emitMockTurnDone();
        },
        async ApproveTab(_tabID, id, allow, session, persist) {
          await withMockTabScope(_tabID, () => this.Approve(id, allow, session, persist));
        },
        async AnswerQuestion(_id, answers) {
      if (!pendingAskPreview) return;
      pendingAskPreview = false;
      const summary = answers
        .map((answer) => `${answer.questionId}: ${(answer.selected ?? []).join(", ") || "(no answer)"}`)
        .join("\n");
      emit({ kind: "message", text: `ask preview answered:\n\n${summary}` });
          emitMockTurnDone();
        },
        async AnswerQuestionForTab(_tabID, id, answers) {
          await withMockTabScope(_tabID, () => this.AnswerQuestion(id, answers));
        },
        async ReplayPendingPrompts() {},
        async ConfirmAction(req) {
          void req;
          return false;
        },
        async SetPlanMode(on) {
          const active = mockTabs.find((tab) => tab.active);
          if (active) await this.SetModeForTab(active.id, modeWithPlan(normalizeMode(active.mode), on));
        },
        async SetMode(mode) {
          const active = mockTabs.find((tab) => tab.active);
          if (active) await this.SetModeForTab(active.id, mode);
        },
        async SetModeForTab(tabID, mode) {
          const nextMode = normalizeMode(mode);
          mockTabs = mockTabs.map((tab) =>
            tab.id === tabID
              ? {
                  ...tab,
                  mode: nextMode,
                  collaborationMode: normalizeCollaborationMode(undefined, tab.goal, nextMode),
                  toolApprovalMode: mockToolApprovalModeAfterModeChange(tab.toolApprovalMode, nextMode),
                }
              : tab,
          );
        },
        async SetCollaborationMode(mode) {
          const active = mockTabs.find((tab) => tab.active);
          if (active) await this.SetCollaborationModeForTab(active.id, mode);
        },
        async SetCollaborationModeForTab(tabID, mode) {
          const next = normalizeCollaborationMode(mode);
          mockTabs = mockTabs.map((tab) => {
            if (tab.id !== tabID) return tab;
            const toolMode = normalizeToolApprovalMode(tab.toolApprovalMode, normalizeMode(tab.mode));
            return {
              ...tab,
              collaborationMode: next,
              goal: next === "normal" || next === "plan" ? "" : tab.goal,
              mode: modeWithPlan(modeWithAutoApproveTools(normalizeMode(tab.mode), toolMode === "yolo"), next === "plan"),
            };
          });
        },
        async SetToolApprovalMode(mode) {
          const active = mockTabs.find((tab) => tab.active);
          if (active) await this.SetToolApprovalModeForTab(active.id, mode);
        },
        async SetToolApprovalModeForTab(tabID, mode) {
          const next = normalizeToolApprovalMode(mode);
          settings.autoApproveTools = next === "yolo";
          settings.bypass = next === "yolo";
          mockTabs = mockTabs.map((tab) =>
            tab.id === tabID
              ? {
                  ...tab,
                  toolApprovalMode: next,
                  mode: modeWithAutoApproveTools(normalizeMode(tab.mode), next === "yolo"),
                }
              : tab,
          );
        },
        async SetGoal(goal) {
          const active = mockTabs.find((tab) => tab.active);
          if (active) await this.SetGoalForTab(active.id, goal);
        },
        async SetGoalForTab(tabID, goal) {
          const nextGoal = goal.trim();
          mockTabs = mockTabs.map((tab) =>
            tab.id === tabID
              ? {
                  ...tab,
                  goal: nextGoal,
                  goalStatus: nextGoal ? "running" : "stopped",
                  collaborationMode: nextGoal ? "goal" : "normal",
                  mode: modeWithPlan(normalizeMode(tab.mode), false),
                }
              : tab,
          );
        },
        async ClearGoal() {
          await this.SetGoal("");
        },
        async ClearGoalForTab(tabID) {
          await this.SetGoalForTab(tabID, "");
        },
        async Compact() {},
        async NewSession() {},
        async ClearSession() {},
    async Checkpoints() {
      return [
        { turn: 0, prompt: "你好呀", files: ["src/App.tsx"], fileCount: 1, turnFileCount: 1, time: Date.now() - 30_000, canCode: true, canConversation: true },
      ];
    },
    async CheckpointsForTab() {
      return this.Checkpoints();
    },
    async Rewind() {},
    async Fork() {
      const active = mockTabs.find((tab) => tab.active) ?? mockTabs[0];
      const tab: TabMeta = {
        ...active,
        id: "tab_fork_" + Date.now(),
        topicId: "topic_fork_" + Date.now(),
        topicTitle: `${active.topicTitle || t("rewind.fork")} · fork`,
        active: true,
        running: false,
      };
      mockTabs = [...mockTabs.map((item) => ({ ...item, active: false })), tab];
      return { ...tab };
    },
    async SummarizeFrom() {},
    async SummarizeUpTo() {},
        async History() {
          return [];
        },
        async HistoryForTab(tabID?: string) {
          const tab = mockTabs.find((item) => item.id === tabID) ?? mockTabs.find((item) => item.active);
          if (tab?.topicId) {
            queueMockTopicRuntime(tab);
            return mockTopicHistory(tab.topicId);
          }
          return this.History();
        },
        async HistoryPage(beforeTurn = 0, limit = 60) {
          return mockHistoryPage(await this.History(), beforeTurn, limit);
        },
        async HistoryPageForTab(tabID: string, beforeTurn = 0, limit = 60) {
          return mockHistoryPage(await this.HistoryForTab(tabID), beforeTurn, limit);
        },
        async HistoryCheckpointTurnsForTab(tabID: string) {
          const turns: number[] = [];
          for (const message of await this.HistoryForTab(tabID)) {
            if (message.role !== "user") continue;
            turns.push(message.checkpointTurn ?? turns.length);
          }
          return turns;
        },
    async ListSessions() {
      return sessions.map((s) => ({ ...s }));
    },
    async ListTrashedSessions() {
      return trashedSessions.map((s) => ({ ...s }));
    },
    async ResumeSession(path: string) {
      sessions.forEach((s) => {
        s.current = s.path === path;
        s.open = s.open || s.path === path;
      });
      return [
        { role: "user", content: `(mock) resumed ${path}` },
        { role: "assistant", content: "This is a mock resumed transcript — the real one comes from the kernel." },
      ];
    },
	    async ResumeSessionForTab(_tabID: string, path: string) {
	      return this.ResumeSession(path);
	    },
	    async ResumeSessionPage(path: string, limit = 60) {
	      return mockHistoryPage(await this.ResumeSession(path), 0, limit);
	    },
	    async ResumeSessionPageForTab(_tabID: string, path: string, limit = 60) {
	      return this.ResumeSessionPage(path, limit);
	    },
	    async OpenChannelSessionForTab(tabID: string, path: string) {
	      mockTabs = mockTabs.map((tab) => tab.id === tabID ? { ...tab, sessionPath: path, readOnly: true } : tab);
	      return this.ResumeSession(path);
	    },
	    async OpenChannelSessionPageForTab(tabID: string, path: string, limit = 60) {
	      return mockHistoryPage(await this.OpenChannelSessionForTab(tabID, path), 0, limit);
	    },
	    async PreviewSession(path: string) {
      const s = sessions.find((x) => x.path === path) ?? trashedSessions.find((x) => x.path === path);
      return [
        { role: "user", content: s?.preview || `(mock) preview ${path}` },
        { role: "phase", content: "Preparing read-only preview" },
        {
          role: "assistant",
          content: "This is a read-only mock preview. The active conversation is unchanged.",
          reasoning: "Preview reads the saved session without resuming it.",
        },
        { role: "notice", level: "info", content: "Preview mode keeps the active conversation untouched." },
        { role: "compaction", content: "", trigger: "manual", messages: 3, summary: "Mock preview preserved the latest task, tool result, and answer summary." },
      ];
    },
    async DeleteSession(path: string) {
      const i = sessions.findIndex((s) => s.path === path);
      if (i >= 0) {
        const [s] = sessions.splice(i, 1);
        trashedSessions.unshift({
          ...s,
          current: false,
          open: false,
          path: s.path.replace("/mock/sessions/", "/mock/sessions/.trash/"),
          deletedAt: Date.now(),
        });
      }
    },
    async RestoreSession(path: string) {
      const i = trashedSessions.findIndex((s) => s.path === path);
      if (i >= 0) {
        const [s] = trashedSessions.splice(i, 1);
        sessions.unshift({
          ...s,
          path: s.path.replace("/mock/sessions/.trash/", "/mock/sessions/"),
          deletedAt: undefined,
        });
      }
    },
    async PurgeTrashedSession(path: string) {
      const i = trashedSessions.findIndex((s) => s.path === path);
      if (i >= 0) trashedSessions.splice(i, 1);
    },
    async RenameSession(path: string, title: string) {
      const s = sessions.find((x) => x.path === path);
      if (s) s.title = title.trim() || undefined;
    },
	    async ScanPromptHistory(nonce: string) {
	      // Dev mock returns a static set of sample prompts for UI development.
	      const entries: PromptHistoryEntry[] = [
	        { text: "Explain the architecture of this project", at: Date.now() - 60000, sessionPath: "/mock/sessions/arch.jsonl", turn: 0 },
	        { text: "Fix the login button styling", at: Date.now() - 120000, sessionPath: "/mock/sessions/arch.jsonl", turn: 1 },
	        { text: "What is the capital of France?", at: Date.now() - 300000, sessionPath: "/mock/sessions/general.jsonl", turn: 0 },
	      ];
	      return { entries, nonce: "mock-" + nonce, olderCursor: "", hasOlder: false };
	    },
    async ListWorkspaces() {
      return mockProjectTree
        .filter((node) => node.kind === "project" && node.root)
        .map((node) => ({
          path: node.root!,
          name: node.label || baseName(node.root!),
          current: node.root === cwd,
        }));
    },
    async PickWorkspace() {
      // Browser dev has no native dialog; simulate picking a folder and re-root so
      // the topbar folder chip visibly changes.
      return mockSwitchWorkspace(cwd.endsWith("another-project") ? "~/projects/reames-agent" : "~/projects/another-project");
    },
    async SwitchWorkspace(path: string) {
      return mockSwitchWorkspace(path);
    },
    async RemoveWorkspace(path: string) {
      workspaces = workspaces.filter((p) => p !== path);
      const index = mockProjectTree.findIndex((node) => node.root === path);
      if (index >= 0) mockProjectTree.splice(index, 1);
    },
        async ContextUsage() {
          return { used: 42124, window: 128000, sessionTokens: 34479, compactRatio: 0.8 };
        },
        async ContextUsageForTab() {
          return this.ContextUsage();
        },
        async Balance() {
      // Mirror the active mock provider: deepseek-flash carries a balance_url.
      const p = settings.providers.find((x) => x.name === settings.defaultModel);
      if (!p?.balanceUrl) return { available: false, display: "" };
          return { available: true, display: "¥128.50" };
        },
        async BalanceForTab() {
          return this.Balance();
        },
        async Jobs() {
          return []; // browser dev mock has no background jobs
        },
        async JobsForTab() {
          return this.Jobs();
        },
        async ToolResultForTab() {
          return null;
        },
        async Meta() {
          const active = mockTabs.find((tab) => tab.active) ?? mockTabs[0];
          const toolApprovalMode = normalizeToolApprovalMode(active?.toolApprovalMode, active ? normalizeMode(active.mode) : "normal", settings.autoApproveTools);
          const autoApproveTools = toolApprovalMode === "yolo";
          const collaborationMode = normalizeCollaborationMode(active?.collaborationMode, active?.goal, active ? normalizeMode(active.mode) : "normal");
          const workspacePath = active?.workspacePath || active?.workspaceRoot || active?.cwd || cwd;
          return {
            label: active?.label ?? "DeepSeek-R1",
            ready: active?.ready ?? true,
            eventChannel: EVENT_CHANNEL,
            cwd: active?.cwd || cwd,
            workspaceRoot: active?.workspaceRoot || workspacePath,
            workspaceName: active?.workspaceName,
            workspacePath,
            sandboxPath: settings.sandbox.workspaceRoot,
            gitBranch: active?.gitBranch || (active?.scope === "project" ? "main" : ""),
            imageInputEnabled: true,
            autoApproveTools,
            bypass: autoApproveTools,
            collaborationMode,
            toolApprovalMode,
            tokenMode: normalizeTokenMode(active?.tokenMode),
            goal: active?.goal ?? "",
            goalStatus: active?.goalStatus ?? (active?.goal ? "running" : "stopped"),
            autoResearch: active?.goal ? { taskId: "mock-autoresearch", status: "running", iteration: 4, pivotRequired: false, staleCount: 0 } : undefined,
          };
        },
        async MetaForTab(tabID) {
          const tab = mockTabs.find((item) => item.id === tabID) ?? mockTabs.find((item) => item.active) ?? mockTabs[0];
          const toolApprovalMode = normalizeToolApprovalMode(tab?.toolApprovalMode, tab ? normalizeMode(tab.mode) : "normal", settings.autoApproveTools);
          const autoApproveTools = toolApprovalMode === "yolo";
          const collaborationMode = normalizeCollaborationMode(tab?.collaborationMode, tab?.goal, tab ? normalizeMode(tab.mode) : "normal");
          const workspacePath = tab?.workspacePath || tab?.workspaceRoot || tab?.cwd || cwd;
          return {
            label: tab?.label ?? "DeepSeek-R1",
            ready: tab?.ready ?? true,
            eventChannel: EVENT_CHANNEL,
            cwd: tab?.cwd || cwd,
            workspaceRoot: tab?.workspaceRoot || workspacePath,
            workspaceName: tab?.workspaceName,
            workspacePath,
            sandboxPath: settings.sandbox.workspaceRoot,
            gitBranch: tab?.gitBranch || (tab?.scope === "project" ? "main" : ""),
            autoApproveTools,
            bypass: autoApproveTools,
            collaborationMode,
            toolApprovalMode,
            tokenMode: normalizeTokenMode(tab?.tokenMode),
            goal: tab?.goal ?? "",
            goalStatus: tab?.goalStatus ?? (tab?.goal ? "running" : "stopped"),
            autoResearch: tab?.goal ? { taskId: "mock-autoresearch", status: "running", iteration: 4, pivotRequired: false, staleCount: 0 } : undefined,
          };
        },
        async AutoResearchCurrent() {
          return {
            taskId: "mock-autoresearch",
            goal: "Mock long-running research",
            status: "running",
            iteration: 4,
            currentDirection: "Inspect status chip",
            staleCount: 0,
            pivotCount: 0,
            pivotRequired: false,
            lastHeartbeatAt: "2026-06-29T00:00:00Z",
            findingCount: 1,
            openCriteria: [],
            blocker: "",
            taskPath: "/tmp/mock/.reames-agent/autoresearch/mock-autoresearch",
            nextRequiredAction: "continue with the next evidence-producing step",
          };
        },
        async AutoResearchStatus(_tabID) {
          return {
            taskId: "mock-autoresearch",
            goal: "Mock long-running research",
            status: "running",
            iteration: 4,
            currentDirection: "Inspect status chip",
            staleCount: 0,
            pivotCount: 0,
            pivotRequired: false,
            lastHeartbeatAt: "2026-06-29T00:00:00Z",
            findingCount: 1,
            openCriteria: [],
            blocker: "",
            taskPath: "/tmp/mock/.reames-agent/autoresearch/mock-autoresearch",
            nextRequiredAction: "continue with the next evidence-producing step",
          };
        },
        async AutoResearchList(_tabID) {
          return [{
            taskId: "mock-autoresearch",
            goal: "Mock long-running research",
            status: "running",
            iteration: 4,
            currentDirection: "Inspect status chip",
            staleCount: 0,
            pivotCount: 0,
            pivotRequired: false,
            lastHeartbeatAt: "2026-06-29T00:00:00Z",
            findingCount: 1,
            openCriteria: [],
            blocker: "",
            taskPath: "/tmp/mock/.reames-agent/autoresearch/mock-autoresearch",
            nextRequiredAction: "continue with the next evidence-producing step",
          }];
        },
        async AutoResearchFindings(_tabID, limit) {
          return [{
            id: "f1",
            kind: "test",
            summary: "Mock accepted finding",
            source: "command",
            command: "go test ./...",
            accepted: true,
            createdAt: "2026-06-29T00:00:00Z",
          }].slice(0, Math.max(0, limit || 1));
        },
        async AutoResearchOpenTask(_tabID) {
          console.info("mock AutoResearchOpenTask");
        },
        async AutoResearchRecordEvidence(_tabID, _criterionID, _input) {
          console.info("mock AutoResearchRecordEvidence");
        },
    async Commands() {
      return [
        { name: "new", description: "start new session; save transcript", kind: "builtin" as const },
        { name: "clear", description: "discard current context", kind: "builtin" as const },
        { name: "compact", description: "Summarize older history to free up context", kind: "builtin" as const },
        { name: "model", description: "Switch model", kind: "builtin" as const },
        { name: "effort", description: "Set reasoning effort", kind: "builtin" as const },
        { name: "skill", description: "List skills", kind: "builtin" as const },
        { name: "plugins", description: "Manage plugin packages", kind: "builtin" as const },
        { name: "explore", description: "Investigate the codebase in an isolated subagent", kind: "skill" as const },
        { name: "review", description: "Review the staged diff", hint: "[focus]", kind: "custom" as const },
      ];
    },
    async Capabilities() {
      return {
        servers: capServers.map((s) => ({ ...s })),
        skills: capSkills.map((s) => ({ ...s })),
        skillRoots: capSkillRoots.map((s) => ({ ...s })),
        plugins: capPlugins.map((p) => ({ ...p })),
      };
    },
    async MCPServers() {
      return capServers.map((s) => ({ ...s }));
    },
    async SkillsSettings() {
      return {
        skills: capSkills.map((s) => ({ ...s })),
        skillRoots: capSkillRoots.map((s) => ({ ...s })),
      };
    },
    async Plugins() {
      return capPlugins.map((p) => ({ ...p }));
    },
    async SearchPluginRegistry(query: string) {
      const entries = [{
        name: "superpowers", description: "Planning and execution workflows", version: "5.1.0",
        author: "obra", category: "workflow", source: "https://github.com/obra/superpowers",
        revision: "d72560e462a74e10d161b7f993d5fc3282bfa1e2",
        digest: "sha256-git-tree-v1:26bcf3b5d0eafe546bdd843185960c65aef576ebc7a1ee8d530c2f8487de7e79",
        permissions: ["hooks.context", "hooks.execute", "skills.load"], registryName: "mock-signed-registry",
        registryMetadataUrl: "https://registry.example/metadata", registryRootVersion: 1,
        registryRootDigest: `sha256:${"a".repeat(64)}`, registryEntryDigest: `sha256:${"c".repeat(64)}`,
        provenanceStatus: "registry-assertion-tuf-authenticated",
      }];
      const needle = query.trim().toLowerCase();
      return entries.filter((entry) => !needle || `${entry.name} ${entry.description} ${entry.category}`.toLowerCase().includes(needle));
    },
    async PluginRegistryEntry(name: string) {
      if (name !== "superpowers") throw new Error(`plugin ${name} is not present in the configured registry`);
      return {
        name: "superpowers", description: "Planning and execution workflows", version: "5.1.0",
        author: "obra", category: "workflow", source: "https://github.com/obra/superpowers",
        revision: "d72560e462a74e10d161b7f993d5fc3282bfa1e2",
        digest: "sha256-git-tree-v1:26bcf3b5d0eafe546bdd843185960c65aef576ebc7a1ee8d530c2f8487de7e79",
        permissions: ["hooks.context", "hooks.execute", "skills.load"], registryName: "mock-signed-registry",
        registryMetadataUrl: "https://registry.example/metadata", registryRootVersion: 1,
        registryRootDigest: `sha256:${"a".repeat(64)}`, registryEntryDigest: `sha256:${"c".repeat(64)}`,
        provenanceStatus: "tuf-attestation-target-integrity-verified",
        attestationDigest: `sha256:${"b".repeat(64)}`,
      };
    },
    async PlanPluginInstall(source: string, options: PluginInstallOptions) {
      const name = options.name || source.split("/").filter(Boolean).pop()?.replace(/\.git$/, "") || "plugin";
      return mockPluginPlan("install", name, "install_plugin_package", source);
    },
    async InstallPlugin(source: string, options: PluginInstallOptions) {
      const name = options.name || source.split("/").filter(Boolean).pop()?.replace(/\.git$/, "") || "plugin";
      const plan = mockPluginPlan("install", name, "install_plugin_package", source);
      if (options.planId !== plan.planId) throw new Error("plugin apply requires the matching preview planId");
      const existing = capPlugins.findIndex((p) => p.name === name);
      const view: PluginView = {
        name,
        version: "dev",
        description: "Mock plugin",
        source,
        root: `~/.reames-agent/plugins/${name}`,
        manifestKind: "reames-agent",
        manifestSchema: 1,
        installMode: options.link ? "link" : "copy",
        sourceKind: source.startsWith("git:") ? "github" : "local-directory",
        trustStatus: source.startsWith("git:") ? "github-https-unsigned" : "local-snapshot",
        digest: `sha256:mock-${name}`,
        permissions: ["skills.load"],
        grantedPermissions: [],
        lifecycleSecurity: 1,
        enabled: false,
        skills: 1,
        hooks: 0,
        mcpServers: 0,
        skillDetails: [{ name: "plan", description: "Plan work before implementation", invocation: "/plan", runAs: "inline" }],
      };
      if (existing >= 0) capPlugins[existing] = view;
      else capPlugins.push(view);
      return mockPluginDone(plan);
    },
    async PlanPluginRemove(name: string) {
      return mockPluginPlan("uninstall", name, "uninstall_plugin_package");
    },
    async RemovePlugin(name: string, planId: string) {
      const plan = mockPluginPlan("uninstall", name, "uninstall_plugin_package");
      if (planId !== plan.planId) throw new Error("plugin removal requires the matching preview planId");
      capPlugins = capPlugins.filter((p) => p.name !== name);
      return mockPluginDone(plan);
    },
    async SetPluginEnabled(name: string, enabled: boolean, expectedDigest: string, grantedPermissions: string[]) {
      const plugin = capPlugins.find((p) => p.name === name);
      if (enabled && (!plugin || plugin.digest !== expectedDigest || JSON.stringify(plugin.permissions || []) !== JSON.stringify(grantedPermissions))) {
        throw new Error("plugin content or permissions changed after approval");
      }
      capPlugins = capPlugins.map((p) => p.name === name ? { ...p, enabled } : p);
    },
    async PlanPluginUpdate(name: string) {
      const plugin = capPlugins.find((p) => p.name === name);
      return mockPluginPlan("install", name, "update_plugin_package", plugin?.source || "");
    },
    async UpdatePlugin(name: string, planId: string) {
      const plugin = capPlugins.find((p) => p.name === name);
      const plan = mockPluginPlan("install", name, "update_plugin_package", plugin?.source || "");
      if (planId !== plan.planId) throw new Error("plugin update requires the matching preview planId");
      capPlugins = capPlugins.map((p) => p.name === name ? {
        ...p,
        version: p.version || "dev",
        rollback: { version: p.version, digest: p.digest, trustStatus: p.trustStatus, permissions: p.permissions, grantedPermissions: p.grantedPermissions, enabled: p.enabled },
      } : p);
      return mockPluginDone(plan);
    },
    async PlanPluginRollback(name: string) {
      return mockPluginPlan("rollback", name, "rollback_plugin_package");
    },
    async RollbackPlugin(name: string, planId: string) {
      const plan = mockPluginPlan("rollback", name, "rollback_plugin_package");
      if (planId !== plan.planId) throw new Error("plugin rollback requires the matching preview planId");
      capPlugins = capPlugins.map((p) => p.name === name && p.rollback ? {
        ...p,
        version: p.rollback.version,
        digest: p.rollback.digest,
        trustStatus: p.rollback.trustStatus,
        permissions: p.rollback.permissions,
        grantedPermissions: p.rollback.grantedPermissions,
        enabled: p.rollback.enabled,
      } : p);
      return mockPluginDone(plan);
    },
    async PluginDoctor(name: string) {
      return capPlugins.find((p) => p.name === name) || {
        name,
        root: "",
        enabled: false,
        skills: 0,
        hooks: 0,
        mcpServers: 0,
        error: "plugin is not installed",
      };
    },
    async AddMCPServer(input: MCPServerInput) {
      const tools = input.transport === "stdio" ? 3 : 5;
      capServers.push({
        name: input.name,
        transport: input.transport,
        status: "connected",
        configured: true,
        autoStart: true,
        tier: "background",
        command: input.command,
        args: input.args,
        url: input.url,
        envKeys: input.env ? Object.keys(input.env).sort() : undefined,
        headerKeys: input.headers ? Object.keys(input.headers).sort() : undefined,
        tools,
        prompts: 0,
        resources: 0,
        toolList: Array.from({ length: tools }, (_, i) => ({
          name: `${input.name}_tool_${i + 1}`,
          description: `Mock tool ${i + 1} exposed by ${input.name}.`,
        })),
      });
      return tools;
    },
    async UpdateMCPServer(name: string, input: MCPServerInput) {
      capServers = capServers.map((s) => {
        if (s.name !== name) return s;
        const connected = s.status === "connected" || s.status === "failed" || s.autoStart !== false;
        const nextStatus = s.status === "disabled" ? "disabled" : connected ? "connected" : "deferred";
        const nextTools = nextStatus === "connected" ? s.tools || (input.transport === "stdio" ? 3 : 5) : 0;
        return {
          ...s,
          transport: input.transport,
          status: nextStatus,
          command: input.transport === "stdio" ? input.command : "",
          args: input.transport === "stdio" ? input.args : [],
          url: input.transport === "stdio" ? "" : input.url,
          envKeys: input.env ? Object.keys(input.env).sort() : s.envKeys,
          headerKeys: input.headers ? Object.keys(input.headers).sort() : s.headerKeys,
          trustedReadOnlyTools: input.trustedReadOnlyTools ?? s.trustedReadOnlyTools,
          tools: nextTools,
          error: undefined,
          authStatus: nextStatus !== "connected" && input.transport !== "stdio" ? "possible" : undefined,
          authUrl: nextStatus !== "connected" && input.transport !== "stdio" ? input.url : undefined,
        };
      });
    },
    async RemoveMCPServer(name: string) {
      capServers = capServers.filter((s) => s.name !== name);
    },
    async ReconnectMCPServer(name: string) {
      capServers = capServers.map((s) =>
        s.name === name
          ? { ...s, status: "initializing", error: undefined, authStatus: undefined, authUrl: undefined }
          : s,
      );
      await new Promise((r) => setTimeout(r, 400));
      capServers = capServers.map((s) =>
        s.name === name ? { ...s, status: "connected", tools: s.tools || 4 } : s,
      );
    },
    async ClearMCPServerAuthentication(name: string) {
      capServers = capServers.map((s) =>
        s.name === name
          ? {
              ...s,
              status: s.autoStart === false ? "disabled" : "initializing",
              tools: 0,
              error: undefined,
              authStatus: s.transport !== "stdio" ? "possible" : undefined,
              authUrl: s.transport !== "stdio" ? s.url : undefined,
              authConfigured: undefined,
            }
          : s,
      );
    },
    async TrustMCPServerTool(name: string, toolName: string) {
      const normalizedTool = toolName.trim();
      if (!normalizedTool) return;
      capServers = capServers.map((s) => {
        if (s.name !== name) return s;
        const trusted = Array.from(new Set([...(s.trustedReadOnlyTools ?? []), normalizedTool]));
        return { ...s, trustedReadOnlyTools: trusted };
      });
    },
    async TrustMCPServerTools(name: string, toolNames: string[]) {
      const normalizedTools = toolNames.map((tool) => tool.trim()).filter(Boolean);
      if (normalizedTools.length === 0) return;
      capServers = capServers.map((s) => {
        if (s.name !== name) return s;
        const trusted = Array.from(new Set([...(s.trustedReadOnlyTools ?? []), ...normalizedTools]));
        return { ...s, trustedReadOnlyTools: trusted };
      });
    },
    async UntrustMCPServerTool(name: string, toolName: string) {
      const normalizedTool = toolName.trim();
      if (!normalizedTool) return;
      capServers = capServers.map((s) => {
        if (s.name !== name) return s;
        const trusted = (s.trustedReadOnlyTools ?? []).filter((tool) => tool !== normalizedTool);
        return { ...s, trustedReadOnlyTools: trusted };
      });
    },
    async PickSkillFolder() {
      return "~/my-skills";
    },
    async PickPluginFolder() {
      return "~/plugins/superpowers";
    },
    async AddSkillPath(path: string) {
      const dir = path.trim() || "~/my-skills";
      if (!capSkillRoots.some((r) => r.scope === "custom" && r.dir === dir)) {
        capSkillRoots.push({
          dir,
          scope: "custom",
          priority: capSkillRoots.length + 1,
          status: "ok",
          configured: true,
          removable: true,
          skills: 1,
          skillItems: [{ name: "local-dev", description: "Local custom development workflow", scope: "custom", runAs: "inline" }],
        });
      }
      if (!capSkills.some((s) => s.name === "local-dev")) {
        capSkills.push({ name: "local-dev", description: "Local custom development workflow", scope: "custom", runAs: "inline", enabled: true });
      }
    },
    async RemoveSkillPath(path: string) {
      capSkillRoots = capSkillRoots.filter((r) => r.dir !== path);
      if (!capSkillRoots.some((r) => r.scope === "custom")) {
        const idx = capSkills.findIndex((s) => s.name === "local-dev");
        if (idx >= 0) capSkills.splice(idx, 1);
      }
    },
    async RefreshSkills() {},
    async ReloadCommands() {},
    async SetSkillEnabled(name: string, enabled: boolean) {
      const skill = capSkills.find((s) => s.name === name);
      if (skill) skill.enabled = enabled;
    },
    async SetMCPServerEnabled(name: string, enabled: boolean) {
      capServers = capServers.map((s) =>
        s.name === name
          ? {
              ...s,
              status: enabled ? "connected" : "disabled",
              autoStart: s.builtIn ? enabled : s.autoStart,
              tools: enabled ? s.tools || 4 : 0,
              error: undefined,
              authStatus: !enabled && s.transport !== "stdio" ? "possible" : undefined,
              authUrl: !enabled && s.transport !== "stdio" ? s.url : undefined,
            }
          : s,
      );
    },
    async SetMCPServerTier(name: string, tier: string) {
      capServers = capServers.map((s) => {
        if (s.name !== name) return s;
        const tools = s.tools || (s.transport === "stdio" ? 3 : 5);
        return { ...s, tier, autoStart: true, status: "connected", tools, error: undefined, authStatus: undefined, authUrl: undefined };
      });
    },
    async SlashArgs(input: string) {
      // Mirror a slice of the real arg hints so the menu is exercisable in browser dev.
      const from = input.lastIndexOf(" ") + 1;
      const cur = input.slice(from);
      const cmd = input.slice(0, input.indexOf(" ") < 0 ? input.length : input.indexOf(" "));
      const subs: Record<string, { label: string; insert: string; hint: string; descend?: boolean }[]> = {
        "/skill": [
          { label: "list", insert: "list", hint: "list skills" },
          { label: "show", insert: "show ", hint: "show a skill's body", descend: true },
          { label: "enable", insert: "enable ", hint: "enable a disabled skill", descend: true },
          { label: "disable", insert: "disable ", hint: "disable an enabled skill", descend: true },
          { label: "new", insert: "new ", hint: "scaffold a new skill" },
          { label: "paths", insert: "paths", hint: "show discovery paths" },
        ],
        "/hooks": [
          { label: "list", insert: "list", hint: "list active hooks" },
          { label: "trust", insert: "trust", hint: "trust this project's hooks" },
        ],
        "/model": [
          { label: "deepseek/deepseek-v4-flash", insert: "deepseek/deepseek-v4-flash", hint: "current" },
          { label: "deepseek/deepseek-v4-pro", insert: "deepseek/deepseek-v4-pro", hint: "" },
        ],
        "/effort": [
          { label: "auto", insert: "auto", hint: "use the model default" },
          { label: "high", insert: "high", hint: "deeper reasoning" },
          { label: "max", insert: "max", hint: "maximum reasoning" },
        ],
      };
      const items = (subs[cmd] ?? [])
        .filter((it) => it.label.toLowerCase().startsWith(cur.toLowerCase()))
        .map((it) => ({ label: it.label, insert: it.insert, hint: it.hint, descend: it.descend ?? false }));
      return { items, from };
    },
    async ListDir(rel: string) {
      // A tiny fake tree so the @ menu is navigable in browser dev.
      if (rel === "" || rel === "./") {
        return [
          { name: "internal", isDir: true },
          { name: "desktop", isDir: true },
          { name: "README.md", isDir: false },
          { name: "go.mod", isDir: false },
        ];
      }
      if (rel === "internal/") {
        return [
          { name: "control", isDir: true },
          { name: "boot", isDir: true },
          { name: "event.go", isDir: false },
        ];
      }
      return [{ name: "file.go", isDir: false }];
    },
    async SearchFileRefs(query: string) {
      const q = query.toLowerCase();
      return ["desktop/frontend/src/lib/bridge.ts", "frontend/wailsjs/runtime/runtime.js", "internal/control/refs.go"]
        .filter((path) => path.split("/").pop()?.toLowerCase().includes(q))
        .map((name) => ({ name, isDir: false }));
    },
    async ReadFile(rel: string) {
      const samples: Record<string, string> = {
        "README.md": "# Reames Agent\n\nBrowser-dev workspace preview.\n\n- Chat in the center\n- Browse files on the right\n- Keep sessions on the left\n",
        "go.mod": "module reames-agent\n\ngo 1.23\n",
        "desktop/file.go": "package desktop\n\nfunc main() {\n\tprintln(\"workspace preview\")\n}\n",
        "internal/event.go": "package internal\n\n// mock file used by the browser dev seam\n",
      };
      return {
        path: rel,
        body: samples[rel] ?? `// ${rel}\n\nMock file body from browser dev.`,
        size: samples[rel]?.length ?? 42,
        truncated: false,
        binary: false,
      };
    },
    async WorkspaceChanges(_tabID: string) {
      return {
        gitAvailable: true,
        gitBranch: "main",
        files: [
          {
            path: "desktop/frontend/src/components/WorkspacePanel.tsx",
            sources: ["session", "git"],
            gitStatus: "M",
            turns: [0, 2],
            latestPrompt: "Mock session edited the workspace panel.",
            latestTime: Date.now() - 60_000,
          },
          { path: "README.md", sources: ["git"], gitStatus: "??" },
          { path: "internal/control/controller.go", sources: ["session"], turns: [1], latestTime: Date.now() - 120_000 },
        ],
      };
    },
    async GitBranches() {
      return ["main", "dev", "feature/branch-switcher"];
    },
    async GitCheckout(_branch: string) {
      console.info("mock GitCheckout", _branch);
    },
    async WorkspaceGitHistory(_tabID: string, path: string) {
      return [
        { hash: "abcdef123456", author: "Mock Author", date: new Date().toISOString(), message: "Mock commit message for " + path },
      ];
    },
    async WorkspaceGitCommitDetail(_tabID: string, _hash: string, path: string) {
      if (path) {
        return { diff: "--- a/mock\n+++ b/mock\n@@ -1,1 +1,1 @@\n-mock\n+mock diff" };
      }
      return { files: ["mock_file_1.ts", "mock_file_2.ts"] };
    },
    async OpenWorkspacePath(rel: string) {
      console.info("mock OpenWorkspacePath", rel);
    },
    async RevealWorkspacePath(rel: string) {
      console.info("mock RevealWorkspacePath", rel);
    },
    async RevealPath(path: string) {
      console.info("mock RevealPath", path);
    },
    async SavePastedImage(dataUrl: string) {
      const path = `.reames-agent/attachments/mock-${mockAttachmentDataURLs.size + 1}.png`;
      mockAttachmentDataURLs.set(path, dataUrl);
      return path;
    },
    async SaveClipboardImage() {
      const path = `.reames-agent/attachments/mock-clipboard-${mockAttachmentDataURLs.size + 1}.png`;
      mockAttachmentDataURLs.set(path, mockPreviewImageDataURL);
      return path;
    },
    async SavePastedFile(name: string, dataUrl: string) {
      const path = `.reames-agent/attachments/mock-${name}`;
      mockAttachmentDataURLs.set(path, dataUrl);
      return path;
    },
    async PickExportFile(defaultFilename: string, _mimeType: string) {
      return defaultFilename;
    },
    async SaveExportFile(path: string, payload: string, base64Encoded: boolean) {
      const a = document.createElement("a");
      let url = "";
      if (base64Encoded) {
        url = `data:application/octet-stream;base64,${payload}`;
      } else {
        url = URL.createObjectURL(new Blob([payload], { type: "text/plain;charset=utf-8" }));
      }
      a.href = url;
      a.download = path;
      document.body.appendChild(a);
      a.click();
      a.remove();
      if (!base64Encoded) URL.revokeObjectURL(url);
    },
    async AttachDropped(path: string) {
      const name = path.split(/[/\\]/).filter(Boolean).pop() ?? path;
      const hasExt = /\.\w{1,6}$/i.test(name);
      if (!hasExt) {
        const tokenName = name.replace(/[^\w.-]+/g, "-") || "folder";
        return { kind: "workspace" as const, path: `__reames-agent_external_folder/mock/${tokenName}`, isDir: true, displayPath: path };
      }
      const attachmentPath = `.reames-agent/attachments/mock-${name}`;
      mockAttachmentDataURLs.set(attachmentPath, mockPreviewImageDataURL);
      return { kind: "attachment" as const, path: attachmentPath };
    },
    async AttachmentDataURL(path: string) {
      return mockAttachmentDataURLs.get(path) ?? mockPreviewImageDataURL;
    },
        async Models() {
          const active = mockTabs.find((tab) => tab.active) ?? mockTabs[0];
          const current = mockTabModelRef(active);
          return mockModelCatalog.map((model) => ({ ...model, current: model.ref === current }));
        },
        async ModelsForTab(tabID) {
          const tab = mockTabs.find((item) => item.id === tabID) ?? mockTabs.find((item) => item.active) ?? mockTabs[0];
          const current = mockTabModelRef(tab);
          return mockModelCatalog.map((model) => ({ ...model, current: model.ref === current }));
        },
        async SetModel(name) {
          setMockTabModel(undefined, name);
        },
        async SetModelForTab(tabID, name) {
          setMockTabModel(tabID, name);
        },
        async Effort() {
          return { supported: true, current: mockEffort, default: "high", levels: ["auto", "high", "max"] };
        },
        async EffortForTab() {
          return this.Effort();
        },
        async SetEffort(level: string) {
          mockEffort = level || "auto";
        },
        async SetEffortForTab(_tabID, level) {
          await this.SetEffort(level);
        },
        async SetTokenMode(mode: string) {
          const active = mockTabs.find((tab) => tab.active);
          if (active) await this.SetTokenModeForTab(active.id, mode);
        },
        async SetTokenModeForTab(tabID, mode) {
          const tokenMode = normalizeTokenMode(mode);
          mockTabs = mockTabs.map((tab) => (tab.id === tabID ? { ...tab, tokenMode } : tab));
        },
    async Memory() {
      return {
        available: true,
        storeDir: "~/.config/reames-agent/projects/-mock/memory",
        storeGlobalDir: "~/.config/reames-agent/memory/global",
        docs: [
          {
            path: "REASONIX.md",
            scope: "project",
            body: "# Reames Agent project memory\n\nMock doc shown in the browser dev seam.\n\n## Notes\n\n- prefers concise replies",
          },
          {
            path: "~/.config/reames-agent/REASONIX.md",
            scope: "user",
            body: t("mock.memoryBody"),
          },
        ],
        facts: [
          {
            name: "prefers-tabs",
            description: "User prefers tabs",
            type: "user",
            body: "Indent with tabs.",
          },
        ],
        archives: [
          {
            name: "old-plan",
            description: "Superseded planning note",
            type: "project",
            body: "This plan was archived after the implementation changed.",
            path: "~/.config/reames-agent/projects/-mock/memory/.archive/20260612-021500.000-old-plan.md",
            archivedAt: "2026-06-12T02:15:00Z",
          },
        ],
        scopes: [
          { scope: "user", path: "~/.config/reames-agent/REASONIX.md" },
          { scope: "project", path: "REASONIX.md" },
          { scope: "local", path: "REASONIX.local.md" },
        ],
      };
    },
    async MemorySuggestions() {
      return {
        memories: [
          {
            id: "memory-prefers-concise-replies",
            name: "prefers-concise-replies",
            title: "Prefers concise replies",
            description: "User prefers concise replies unless detail is requested.",
            type: "user",
            body: "User prefers concise replies unless detail is requested.\n\n**Why:** Suggested from recent local history.\n**How to apply:** Keep answers brief by default.",
            reason: "future-facing preference",
            evidence: ["mock-session: always keep replies concise"],
          },
        ],
        skills: [
          {
            id: "skill-reames-agent-pr-followup",
            name: "reames-agent-pr-followup",
            description: "Review or update a Reames Agent GitHub PR, address feedback, verify, and publish safely.",
            scope: "project",
            body: "# Reames Agent PR Followup\n\nUse this skill for repeated Reames Agent PR work.\n\n## Workflow\n\n1. Confirm branch and PR state.\n2. Inspect the diff.\n3. Fix actionable feedback.\n4. Verify and update the PR.\n",
            reason: "recent history repeatedly touched PR workflows",
            evidence: ["mock-pr-session: 提交到pr，并更新内容", "mock-review-session: 解决该pr下机器人提出来的问题"],
          },
        ],
        generatedAt: new Date().toISOString(),
        available: true,
        source: "mock",
      };
    },
    async AcceptMemorySuggestion(suggestion: MemorySuggestion) {
      emit({ kind: "notice", level: "info", text: `saved suggested memory → ${suggestion.name}` });
      return `${suggestion.name}.md`;
    },
    async AcceptSkillSuggestion(suggestion: SkillSuggestion) {
      emit({ kind: "notice", level: "info", text: `created suggested skill → ${suggestion.name}` });
      return `.reames-agent/skills/${suggestion.name}/SKILL.md`;
    },
    async MemorySuggestionsForTab(_tabID: string) {
      return this.MemorySuggestions();
    },
    async AcceptMemorySuggestionForTab(_tabID: string, suggestion: MemorySuggestion) {
      return this.AcceptMemorySuggestion(suggestion);
    },
    async AcceptSkillSuggestionForTab(_tabID: string, suggestion: SkillSuggestion) {
      return this.AcceptSkillSuggestion(suggestion);
    },
    async MemoryForTab(_tabID: string) {
      return this.Memory();
    },
    async Remember(_scope: string, _note: string) {
      emit({ kind: "notice", level: "info", text: `remembered → ${_scope}` });
      return `${_scope} REASONIX.md (mock): ${_note}`;
    },
    async RememberForTab(_tabID: string, scope: string, note: string) {
      return this.Remember(scope, note);
    },
    async Forget(_name: string) {
      emit({ kind: "notice", level: "info", text: `forgot → ${_name}` });
    },
    async ForgetForTab(_tabID: string, name: string) {
      return this.Forget(name);
    },
    async SaveDoc(_path: string, _body: string) {
      emit({ kind: "notice", level: "info", text: `saved → ${_path}` });
      return _path;
    },
    async SaveDocForTab(_tabID: string, path: string, body: string) {
      return this.SaveDoc(path, body);
    },
    async DesktopStartupSettings() {
      const { bot, desktopLanguage, desktopLayoutStyle, desktopTheme, desktopThemeStyle, displayMode, statusBarStyle, statusBarItems, checkUpdates } = settings;
      return JSON.parse(JSON.stringify({
        bot,
        desktopLanguage,
        desktopLayoutStyle,
        desktopTheme,
        desktopThemeStyle,
        displayMode,
        statusBarStyle,
        statusBarItems,
        checkUpdates,
      })) as DesktopStartupSettingsView;
    },
    async Settings() {
      return JSON.parse(JSON.stringify(settings)) as SettingsView;
    },
    async HooksSettings(scope: string) {
      const key = scope === "project" ? "project" : "global";
      return JSON.parse(JSON.stringify(hookSettings[key])) as HooksSettingsView;
    },
    async SaveHooksSettings(scope: string, hooks: HookConfigView[]) {
      const key = scope === "project" ? "project" : "global";
      hookSettings[key].hooks = JSON.parse(JSON.stringify(hooks)) as HookConfigView[];
    },
    async SaveHooksSettingsForRoot(scope: string, _projectRoot: string, hooks: HookConfigView[]) {
      const key = scope === "project" ? "project" : "global";
      hookSettings[key].hooks = JSON.parse(JSON.stringify(hooks)) as HookConfigView[];
    },
    async TrustProjectHooks() {
      hookSettings.project.trusted = true;
    },
    async TrustProjectHooksForRoot(projectRoot: string) {
      if (projectRoot && projectRoot === hookSettings.project.projectRoot) {
        hookSettings.project.trusted = true;
      }
    },
    async SetDefaultModel(ref: string) {
      settings.defaultModel = ref;
    },
    async SetPlannerModel(ref: string) {
      settings.plannerModel = ref;
    },
    async SetSubagentModel(ref: string) {
      settings.subagentModel = ref;
    },
    async SetSubagentEffort(level: string) {
      settings.subagentEffort = level;
    },
    async SetMaxSubagentDepth(depth: number) {
      settings.agent = { ...settings.agent, maxSubagentDepth: depth <= 1 ? 1 : 2 };
    },
    async SetAutoPlan(mode: string) {
      settings.autoPlan = mode;
    },
    async SetDefaultToolApprovalMode(mode: string) {
      settings.defaultToolApprovalMode = normalizeToolApprovalMode(mode);
    },
    async SaveProvider(p: ProviderView) {
      p.added = true;
      const i = settings.providers.findIndex((x) => x.name === p.name);
      if (i >= 0) settings.providers[i] = p;
      else settings.providers.push(p);
    },
    async SaveProviderWithKey(p: ProviderView, key: string) {
      p.added = true;
      p.keySet = Boolean(key.trim()) || p.keySet;
      const i = settings.providers.findIndex((x) => x.name === p.name);
      if (i >= 0) settings.providers[i] = p;
      else settings.providers.push(p);
      return "";
    },
    async AddOfficialProviderAccess(kind: string, key: string) {
      const templates: Record<string, ProviderView> = {
        deepseek: { name: "deepseek", builtIn: true, added: true, kind: "openai", baseUrl: "https://api.deepseek.com", modelsUrl: "", models: ["deepseek-v4-flash", "deepseek-v4-pro"], visionModels: [], visionModelsConfigured: false, default: "deepseek-v4-flash", apiKeyEnv: "DEEPSEEK_API_KEY", keySet: !!key.trim(), balanceUrl: "https://api.deepseek.com/user/balance", contextWindow: 1_000_000, reasoningProtocol: "", thinking: "", supportedEfforts: [], defaultEffort: "" },
      };
      const next = templates[kind];
      if (!next) throw new Error(`unknown official provider template ${kind}`);
      const i = settings.providers.findIndex((x) => x.name === next.name);
      if (i >= 0) settings.providers[i] = { ...settings.providers[i], ...next, keySet: next.keySet || settings.providers[i].keySet };
      else settings.providers.push(next);
      return "";
    },
    async AddProviderPresetAccess(id: string, key: string) {
      const preset = settings.providerPresets.find((p) => p.id === id);
      if (!preset) throw new Error(`unknown provider preset ${id}`);
      const next = cloneMockProviderTemplate(id, key);
      if (!next) throw new Error(`unknown provider preset ${id}`);
      const i = settings.providers.findIndex((x) => x.name === next.name);
      if (i >= 0) settings.providers[i] = { ...settings.providers[i], ...next, keySet: next.keySet || settings.providers[i].keySet };
      else settings.providers.push(next);
      preset.added = true;
      preset.status = "installed";
      preset.statusProviderNames = [...preset.providerNames];
      preset.keySet = preset.keySet || !!key.trim();
      preset.configured = !preset.requiresKey || preset.keySet;
      return "";
    },
    async ResetProviderPresetAccess(id: string) {
      const preset = settings.providerPresets.find((p) => p.id === id);
      if (!preset) throw new Error(`unknown provider preset ${id}`);
      const next = cloneMockProviderTemplate(id, "");
      if (!next) throw new Error(`unknown provider preset ${id}`);
      const i = settings.providers.findIndex((x) => x.name === next.name);
      if (i < 0) throw new Error(`provider preset ${id} cannot be reset because no same-name provider exists`);
      const existing = settings.providers[i];
      settings.providers[i] = {
        ...next,
        added: true,
        keySet: existing.apiKeyEnv === next.apiKeyEnv ? existing.keySet : next.keySet,
      };
      preset.added = true;
      preset.status = "installed";
      preset.statusProviderNames = [...preset.providerNames];
      preset.keySet = preset.keySet || settings.providers[i].keySet;
      preset.configured = !preset.requiresKey || preset.keySet;
    },
    async FetchProviderModels(p: ProviderView) {
      if (!p.baseUrl.trim()) throw new Error(t("settings.fetchModelsMissingBaseUrl"));
      if (providerRequiresKey(p) && !p.apiKeyEnv.trim()) throw new Error(t("settings.fetchModelsMissingKeyEnv"));
      await delay(350);
      if (p.baseUrl.includes("deepseek")) return ["deepseek-v4-flash", "deepseek-v4-pro"];
      if (p.baseUrl.includes("token-plan")) return ["mimo-v2.5", "mimo-v2.5-pro"];
      if (p.baseUrl.includes("xiaomimimo")) return ["mimo-v2.5-pro", "mimo-v2.5"];
      return ["gpt-5", "gpt-5-mini", "qwen3-coder"];
    },
    async DeleteProvider(name: string) {
      settings.providers = settings.providers.filter((p) => p.name !== name);
    },
    async RemoveProviderAccess(name: string) {
      const p = settings.providers.find((x) => x.name === name);
      if (p?.builtIn) p.added = false;
      else settings.providers = settings.providers.filter((x) => x.name !== name);
    },
    async SaveProviderKey(apiKeyEnv: string, _value: string) {
      settings.providers.forEach((p) => {
        if (p.apiKeyEnv === apiKeyEnv) p.keySet = true;
      });
      return "";
    },
    async SetProviderKey(apiKeyEnv: string, _value: string) {
      settings.providers.forEach((p) => {
        if (p.apiKeyEnv === apiKeyEnv) p.keySet = true;
      });
      return "";
    },
    async ClearProviderKey(apiKeyEnv: string) {
      settings.providers.forEach((p) => {
        if (p.apiKeyEnv === apiKeyEnv) p.keySet = false;
      });
    },
    async SetPermissionMode(mode: string) {
      settings.permissions.mode = mode;
    },
    async AddPermissionRule(list: string, rule: string) {
      const k = list as "allow" | "ask" | "deny";
      if (settings.permissions[k] && !settings.permissions[k].includes(rule)) settings.permissions[k].push(rule);
    },
    async RemovePermissionRule(list: string, rule: string) {
      const k = list as "allow" | "ask" | "deny";
      settings.permissions[k] = settings.permissions[k].filter((r) => r !== rule);
    },
        async ReloadSettings() {},
        async SetSandbox(bash: string, network: boolean, workspaceRoot: string, allowWrite: string[], shell: string) {
          const effectiveWorkspaceRoot = workspaceRoot.trim() || cwd;
          settings.sandbox = { bash, network, workspaceRoot, allowWrite, effectiveWorkspaceRoot, effectiveWriteRoots: [effectiveWorkspaceRoot, ...allowWrite], shell };
        },
        async SetNetwork(n: NetworkView) {
          settings.network = n;
        },
        async SetBotSettings(b: BotSettingsView) {
          settings.bot = JSON.parse(JSON.stringify(b)) as BotSettingsView;
        },
        async SetBotConnectionToolApprovalMode(connID, mode) {
          const conn = settings.bot.connections.find((c) => c.id === connID);
          if (conn) conn.toolApprovalMode = mode as any;
        },
        async SetBotSecret(envName: string, _value: string) {
          const name = envName.trim();
          if (settings.bot.qq.appSecretEnv === name) settings.bot.qq.secretSet = true;
          if (settings.bot.feishu.appSecretEnv === name) settings.bot.feishu.secretSet = true;
          if (settings.bot.weixin.tokenEnv === name) settings.bot.weixin.tokenSet = true;
          settings.bot.connections = settings.bot.connections.map((connection) => ({
            ...connection,
            credential: connection.credential.appSecretEnv === name || connection.credential.tokenEnv === name
              ? { ...connection.credential, secretSet: true }
              : connection.credential,
          }));
        },
        async ClearBotSecret(envName: string) {
          const name = envName.trim();
          if (settings.bot.qq.appSecretEnv === name) settings.bot.qq.secretSet = false;
          if (settings.bot.feishu.appSecretEnv === name) settings.bot.feishu.secretSet = false;
          if (settings.bot.weixin.tokenEnv === name) settings.bot.weixin.tokenSet = false;
          settings.bot.connections = settings.bot.connections.map((connection) => ({
            ...connection,
            credential: connection.credential.appSecretEnv === name || connection.credential.tokenEnv === name
              ? { ...connection.credential, secretSet: false }
              : connection.credential,
          }));
        },
        async BotRuntimeStatus() {
          const qqRunning = settings.bot.qq.enabled && settings.bot.qq.appId.trim() && settings.bot.qq.secretSet;
          const runningConnections = (qqRunning ? 1 : 0) + settings.bot.connections.filter((connection) => connection.enabled && connection.status === "connected").length;
          return {
            running: settings.bot.enabled && runningConnections > 0,
            status: settings.bot.enabled && runningConnections > 0 ? "running" : "stopped",
            message: settings.bot.enabled && runningConnections > 0 ? `${runningConnections} bot connection(s) running` : "bot runtime is not started",
            connections: runningConnections,
            startedAt: settings.bot.enabled && runningConnections > 0 ? new Date(t0).toISOString() : "",
          };
        },
        async StartBotConnectionInstall(provider: string, domain: string) {
          const normalizedProvider = provider === "weixin" ? "weixin" : "feishu";
          const normalizedDomain = normalizedProvider === "weixin" ? "weixin" : domain === "lark" ? "lark" : "feishu";
          return {
            ok: true,
            provider: normalizedProvider,
            domain: normalizedDomain,
            installId: `mock-${normalizedProvider}-${normalizedDomain}`,
            url: "https://example.com/reames-agent-bot-qr",
            deviceCode: "MOCKDEVICE",
            userCode: normalizedProvider === "weixin" ? "" : "MOCK-CODE",
            interval: 3,
            expireIn: 300,
            message: "",
          };
        },
        async PollBotConnectionInstall(installID: string) {
          const isWeixin = installID.includes("weixin");
          const domain = installID.includes("lark") ? "lark" : isWeixin ? "weixin" : "feishu";
          const provider = isWeixin ? "weixin" : "feishu";
          const connection = {
            id: `${provider}-${domain}`,
            provider,
            domain,
            label: domain === "lark" ? "Lark" : domain === "weixin" ? "微信" : "飞书",
            enabled: true,
            status: "connected",
	            model: "",
	            toolApprovalMode: "",
	            workspaceRoot: "",
	            access: { enabled: true, allowAll: false, pairingEnabled: true, users: [provider === "weixin" ? "wxid_mock_user_001" : "ou_mock_user_001"], groups: [], approvers: [], admins: [] },
	            credential: {
              appId: provider === "feishu" ? "cli_mock" : "",
              appSecretEnv: provider === "feishu" ? (domain === "lark" ? "LARK_BOT_APP_SECRET" : "FEISHU_BOT_APP_SECRET") : "",
              accountId: provider === "weixin" ? "mock-account" : "",
              tokenEnv: provider === "weixin" ? "WEIXIN_BOT_TOKEN" : "",
              secretSet: true,
            },
            sessionMappings: [],
            lastError: "",
            createdAt: new Date().toISOString(),
            updatedAt: new Date().toISOString(),
          };
          settings.bot.connections = [...settings.bot.connections.filter((c) => c.id !== connection.id), connection];
          return { done: true, connection, status: "connected", message: "connected", error: "" };
        },
        async DiagnoseBotConnection(id: string) {
          const connection = settings.bot.connections.find((c) => c.id === id);
          const occurredAt = new Date().toISOString();
          return connection
            ? { id, label: connection.label, status: connection.enabled ? "ok" : "disabled", message: connection.enabled ? "连接配置已保存。" : "连接已保存但未启用。", messageId: "", phase: "config", code: connection.enabled ? "config_ok" : "connection_disabled", reportKind: "", reportDetail: "", occurredAt }
            : { id, label: "", status: "missing", message: "未找到连接。", messageId: "", phase: "config", code: "connection_missing", reportKind: "bot", reportDetail: JSON.stringify({ schemaVersion: 2, kind: "bot", source: "bot.runtime", label: "bot.mock.config", message: "mock missing bot connection", errorType: "BotConnectionDiagnostic", errorMessage: "bot connection record was not found", topFrame: "bot.config", occurredAt }), occurredAt };
        },
        async TestBotConnection(id: string, target?: string) {
          const diag = await this.DiagnoseBotConnection(id);
          if (target?.trim()) return { ...diag, message: `Mock test sent to ${target.trim()}`, messageId: "mock-message-id" };
          return diag;
        },
        async SetCloseBehavior(mode: string) {
          settings.closeBehavior = mode === "quit" ? "quit" : "background";
        },
        async SetDisplayMode(mode: string) {
          settings.displayMode = mode;
        },
        async SetStatusBarStyle(style: string) {
          settings.statusBarStyle = style === "text" ? "text" : "icon";
        },
        async SetStatusBarItems(items: string[]) {
          settings.statusBarItems = normalizeStatusBarItems(items);
        },
        async SetDesktopLanguage(lang: string) {
          settings.desktopLanguage = lang === "en" || lang === "zh" ? lang : "";
        },
        async SetDesktopAppearance(theme: string, style: string) {
          settings.desktopTheme = theme === "auto" || theme === "light" ? theme : "dark";
          settings.desktopThemeStyle = style;
        },
        async SetDesktopLayoutStyle(style: string) {
          settings.desktopLayoutStyle = style === "workbench" || style === "creation" ? style : "classic";
        },
        async SetDesktopZoomFactor(factor: number) {
          mockDesktopZoomFactor = Math.min(2.0, Math.max(0.5, Number.isFinite(factor) ? factor : 1.0));
        },
        async GetDesktopZoomFactor() {
          return mockDesktopZoomFactor;
        },
        async RestartApplication() {
          // no-op in mock
        },
        async SetDesktopCheckUpdates(enabled: boolean) {
          settings.checkUpdates = enabled;
        },
        async SetDesktopTelemetry(enabled: boolean) {
          settings.telemetry = enabled;
        },
        async SetDesktopMetrics(enabled: boolean) {
          settings.metrics = enabled;
        },
        async SetMemoryCompilerEnabled(enabled: boolean) {
          settings.memoryCompilerEnabled = enabled;
        },
        async SetExpandThinking(_on: boolean) {},
        async MigrateDesktopPreferences(language: string, theme: string, style: string) {
          if (!settings.desktopLanguage) settings.desktopLanguage = language === "en" || language === "zh" || language === "zh-TW" ? language : "";
          if (!settings.desktopTheme && !settings.desktopThemeStyle) {
            settings.desktopTheme = theme === "auto" || theme === "light" ? theme : "dark";
            settings.desktopThemeStyle = style;
          }
        },
    async SetAgentParams(temperature: number, maxSteps: number, plannerMaxSteps: number, systemPrompt: string) {
      settings.agent = { ...settings.agent, temperature, maxSteps, plannerMaxSteps, systemPrompt };
    },
    async SetColdResumePrune(enabled: boolean) {
      settings.agent = { ...settings.agent, coldResumePrune: enabled };
    },
    async SetReasoningLanguage(lang: string) {
      const normalized = lang === "zh" || lang === "en" ? lang : "auto";
      settings.agent = { ...settings.agent, reasoningLanguage: normalized };
    },
    // ── Heartbeat mock ──
    async HeartbeatListTasks() { return []; },
    async HeartbeatReloadTasks() { return []; },
    async HeartbeatSaveTasks(_tasks: unknown) {},
    async HeartbeatTriggerNow(_id: string) {},
    async HeartbeatGenerateID() { return "mock-" + Date.now().toString(36); },
    async SetTrayLocale(_locale: "en" | "zh" | "zh-TW") {},
    async SetAutoApproveTools(on: boolean) {
      await this.SetToolApprovalMode(on ? "yolo" : "ask");
    },
    async SetBypass(on: boolean) {
      await this.SetAutoApproveTools(on);
    },
    async Version() {
      return "v1.0.0 (browser dev)";
    },
    async CheckUpdate() {
      // Keep the default browser preview focused on the primary product surface.
      // DownloadUpdate/InstallUpdate remain mocked for explicit updater-flow tests.
      return {
        available: false,
        current: "v1.0.0",
        latest: "v1.0.0",
        notes: "",
        channel: "stable",
        canSelfUpdate: false,
        manualOnly: true,
        manualReason: "browser preview",
        downloaded: false,
        downloadUrl: "",
        assetSize: 0,
      };
    },
    async DownloadUpdate() {
      const total = 12_345_678;
      for (let r = 0; r <= total; r += 1_800_000) {
        emitUpdater({ phase: "downloading", received: Math.min(r, total), total });
        await delay(120);
      }
      emitUpdater({ phase: "verifying", received: total, total });
      await delay(500);
      emitUpdater({ phase: "downloaded", received: total, total });
      return { version: "v1.1.0", channel: "stable", path: "/tmp/reames-agent-update", size: total, sha256: "mock" };
    },
    async InstallUpdate() {
      const total = 12_345_678;
      emitUpdater({ phase: "installing", received: total, total });
      await delay(500);
      emitUpdater({ phase: "done", received: total, total });
      // The real shell relaunches here; the mock just stops.
    },
    async ApplyUpdate() {
      await this.DownloadUpdate();
      await this.InstallUpdate();
    },
    async OpenDownloadPage() {
      if (typeof window !== "undefined") {
        window.open("https://github.com/Ebonyhtx/reames-agent/releases", "_blank", "noopener");
      }
    },
    // Dev seam: drives the overlay flow in the browser until ConnectKey sets the
    // key. Matches ConnectKey on apiKeyEnv so the two stay in sync.
    async NeedsOnboarding() {
      return !onboardingDismissed && !settings.providers.find((p) => p.apiKeyEnv === "DEEPSEEK_API_KEY")?.keySet;
    },
    async DismissOnboarding() {
      onboardingDismissed = true;
    },
    async ConnectKey(apiKey: string) {
      if (!apiKey.trim()) throw new Error("key is required");
      settings.providers.forEach((p) => {
        if (p.apiKeyEnv === "DEEPSEEK_API_KEY") p.keySet = true;
      });
      await delay(300);
      return "";
    },
    async ReportCrash() {
      await delay(300);
    },
    // Tab management mocks.
    async ListTabs() {
      return mockTabs.map((tab) => ({ ...tab }));
    },
    async OpenProjectTab(workspaceRoot: string, _topicID: string) {
      const existing = mockTabs.find((tab) => tab.scope === "project" && tab.workspaceRoot === workspaceRoot && tab.topicId === _topicID);
      if (existing) {
        const active = { ...existing, active: true, running: mockTopicRunsInScenario(_topicID) };
        mockTabs = mockTabs.map((tab) => (tab.id === existing.id ? active : { ...tab, active: false }));
        return { ...active };
      }
      const defaultToolApprovalMode = normalizeToolApprovalMode(settings.defaultToolApprovalMode);
      const tab: TabMeta = {
        id: "tab_" + Date.now(),
        scope: "project",
        workspaceRoot,
        workspaceName: workspaceRoot.split("/").filter(Boolean).pop() ?? workspaceRoot,
        workspacePath: workspaceRoot,
        gitBranch: "main",
        topicId: _topicID,
        topicTitle: topicLabel(_topicID, t("mock.newSession")),
        sessionPath: `/mock/sessions/${_topicID}.jsonl`,
        projectColor: mockProjectTree.find((node) => node.root === workspaceRoot)?.projectColor,
        label: mockModelLabel(settings.defaultModel),
        ready: true,
        running: mockTopicRunsInScenario(_topicID),
        mode: modeWithAutoApproveTools("normal", defaultToolApprovalMode === "yolo"),
        collaborationMode: "normal",
        toolApprovalMode: defaultToolApprovalMode,
        tokenMode: "full",
        active: true,
        cwd: workspaceRoot,
      };
      mockTabs = [...mockTabs.map((item) => ({ ...item, active: false })), tab];
      return { ...tab };
    },
    async OpenGlobalTab(_topicID: string) {
      const existing = mockTabs.find((tab) => tab.scope === "global" && tab.topicId === _topicID);
      if (existing) {
        setMockActiveTab(existing.id);
        return { ...existing, active: true };
      }
      const defaultToolApprovalMode = normalizeToolApprovalMode(settings.defaultToolApprovalMode);
      const tab: TabMeta = {
        id: "tab_" + Date.now(),
        scope: "global",
        workspaceRoot: "",
        workspaceName: "Global",
        workspacePath: cwd,
        topicId: _topicID,
        topicTitle: topicLabel(_topicID, "Global"),
        sessionPath: `/mock/sessions/${_topicID}.jsonl`,
        label: mockModelLabel(settings.defaultModel),
        ready: true,
        running: false,
        mode: modeWithAutoApproveTools("normal", defaultToolApprovalMode === "yolo"),
        collaborationMode: "normal",
        toolApprovalMode: defaultToolApprovalMode,
        tokenMode: "full",
        active: true,
        cwd: "",
      };
      mockTabs = [...mockTabs.map((item) => ({ ...item, active: false })), tab];
      return { ...tab };
    },
    async OpenTopicSession(scope: string, workspaceRoot: string, topicID: string, sessionPath: string) {
      const tab = scope === "project"
        ? await this.OpenProjectTab(workspaceRoot, topicID)
        : await this.OpenGlobalTab(topicID);
      const active = { ...tab, sessionPath };
      mockTabs = mockTabs.map((item) => (item.id === tab.id ? active : item));
      return { ...active };
    },
    async EnsureBlankTab(scope: string, workspaceRoot: string) {
      const targetScope = scope === "project" && workspaceRoot ? "project" : "global";
      const targetRoot = targetScope === "project" ? workspaceRoot : "";
      const existing = mockTabs.find((tab) =>
        tab.scope === targetScope &&
        (targetScope === "global" || tab.workspaceRoot === targetRoot) &&
        !tab.running &&
        mockTopicIsBlank(tab.topicId)
      );
      if (existing) {
        setMockActiveTab(existing.id);
        return { ...existing, active: true };
      }
      const topic = await this.CreateTopic(targetScope, targetRoot, "");
      return targetScope === "global" ? this.OpenGlobalTab(topic.id) : this.OpenProjectTab(targetRoot, topic.id);
    },
    async ActivateTopic(scope: string, workspaceRoot: string, topicID: string, sessionPath: string) {
      const tab = sessionPath
        ? await this.OpenTopicSession(scope, workspaceRoot, topicID, sessionPath)
        : scope === "project"
          ? await this.OpenProjectTab(workspaceRoot, topicID)
          : await this.OpenGlobalTab(topicID);
      mockTabs = mockTabs.filter((item) => item.id === tab.id).map((item) => ({ ...item, active: true }));
      return { ...mockTabs[0] };
    },
    async EnsureBlankSurface(scope: string, workspaceRoot: string) {
      const tab = await this.EnsureBlankTab(scope, workspaceRoot);
      mockTabs = mockTabs.filter((item) => item.id === tab.id).map((item) => ({ ...item, active: true }));
      return { ...mockTabs[0] };
    },
    async SetActiveTab(_tabID: string) {
      setMockActiveTab(_tabID);
      const tab = mockTabs.find((item) => item.id === _tabID);
      if (tab) queueMockTopicRuntime(tab);
    },
    async ReorderTabs(_tabIDs: string[]) {
      const byId = new Map(mockTabs.map((tab) => [tab.id, tab]));
      const ordered = _tabIDs.map((id) => byId.get(id)).filter((tab): tab is TabMeta => Boolean(tab));
      if (ordered.length === mockTabs.length) mockTabs = ordered;
    },
    async CloseTab(_tabID: string) {
      if (mockTabs.length <= 1) return;
      const wasActive = mockTabs.some((tab) => tab.id === _tabID && tab.active);
      mockTabs = mockTabs.filter((tab) => tab.id !== _tabID);
      if (wasActive && mockTabs.length > 0 && !mockTabs.some((tab) => tab.active)) {
        mockTabs[mockTabs.length - 1] = { ...mockTabs[mockTabs.length - 1], active: true };
      }
    },
    async ListProjectTree() {
      return cloneProjectTree();
    },
    async RenameProject(workspaceRoot: string, title: string) {
      const node = workspaceRoot
        ? mockProjectTree.find((item) => item.root === workspaceRoot)
        : mockProjectTree.find((item) => item.kind === "global_folder");
      if (node) node.label = title.trim() || (node.kind === "global_folder" ? "Global" : node.label);
    },
    async SetProjectColor(workspaceRoot: string, color: string) {
      const node = workspaceRoot
        ? mockProjectTree.find((item) => item.root === workspaceRoot)
        : mockProjectTree.find((item) => item.kind === "global_folder");
      if (!node) return;
      node.projectColor = color || undefined;
      for (const child of projectChildren(node)) child.projectColor = node.projectColor;
      mockTabs = mockTabs.map((tab) =>
        (workspaceRoot ? tab.workspaceRoot === workspaceRoot : tab.scope === "global")
          ? { ...tab, projectColor: node.projectColor }
        : tab,
      );
    },
    async SetProjectPinned(workspaceRoot: string, pinned: boolean) {
      setMockProjectPinned(workspaceRoot, pinned);
    },
    async ReorderProjects(workspaceRoots: string[]) {
      const projects = mockProjectTree.filter((node) => node.kind === "project");
      const globals = mockProjectTree.filter((node) => node.kind === "global_folder");
      if (!workspaceRoots.includes(GLOBAL_PROJECT_ORDER_KEY)) {
        if (workspaceRoots.length !== projects.length) return;
        const byRoot = new Map(projects.map((node) => [node.root, node]));
        const ordered = workspaceRoots.map((root) => byRoot.get(root)).filter((node): node is ProjectNode => Boolean(node));
        if (ordered.length !== projects.length) return;
        mockProjectTree.splice(0, mockProjectTree.length, ...globals, ...ordered);
        return;
      }
      const byKey = new Map<string, ProjectNode>();
      for (const node of projects) {
        if (node.root) byKey.set(node.root, node);
      }
      for (const node of globals) byKey.set(GLOBAL_PROJECT_ORDER_KEY, node);
      const seen = new Set<string>();
      const ordered: ProjectNode[] = [];
      for (const key of workspaceRoots) {
        if (seen.has(key)) return;
        const node = byKey.get(key);
        if (!node) return;
        seen.add(key);
        ordered.push(node);
      }
      if (ordered.length !== projects.length + globals.length) return;
      mockProjectTree.splice(0, mockProjectTree.length, ...ordered);
    },
    async CreateTopic(_scope: string, _workspaceRoot: string, title: string) {
      const now = Date.now();
      const id = "topic_" + now;
      const topicTitle = title.trim() || t("mock.newSession");
      const parent = _scope === "global"
        ? ensureMockGlobalFolder()
        : mockProjectTree.find((node) => node.root === _workspaceRoot);
      if (parent) {
        const global = parent.kind === "global_folder";
        parent.children = [{
          key: parent.kind === "global_folder" ? "global_topic_" + id : "topic_" + id,
          kind: global ? "global_topic" : "topic",
          label: topicTitle,
          root: parent.root,
          topicId: id,
          projectColor: parent.projectColor,
          createdAt: now,
        }, ...projectChildren(parent)];
      }
      return { id, title: topicTitle, createdAt: now };
    },
    async RenameTopic(topicID: string, title: string) {
      const topic = findMockTopic(topicID);
      const nextTitle = title.trim();
      if (!topic || !nextTitle) return;
      const activePrefix = topic.label?.startsWith("● ") ? "● " : "";
      topic.label = `${activePrefix}${nextTitle}`;
      mockTabs = mockTabs.map((tab) =>
        tab.topicId === topicID ? { ...tab, topicTitle: nextTitle } : tab,
      );
    },
    async DeleteTopic(topicID: string) {
      deleteMockTopic(topicID);
    },
    async TrashTopic(topicID: string) {
      deleteMockTopic(topicID);
    },
    async SetTopicPinned(topicID: string, pinned: boolean) {
      setMockTopicPinned(topicID, pinned);
    },
    async SaveWindowState(_state) {
      // no-op in browser dev — no real window geometry to persist
    },
    async ContextPanel(_tabID: string) {
      const now = Date.now();
      const currency = "¥";
      const cost = (usd: number) => currency === "¥" ? Number((usd * 7.15).toFixed(4)) : usd;
      return {
        usedTokens: 42124,
        windowTokens: 128000,
        promptTokens: 22134,
        completionTokens: 12345,
        totalTokens: 34479,
        reasoningTokens: 7521,
        cacheHitTokens: 87000,
        cacheMissTokens: 13000,
        sessionCacheHitTokens: 87000,
        sessionCacheMissTokens: 13000,
        sessionCompletionTokens: 12345,
        requestCount: 10,
        elapsedMs: 33 * 60 * 1000,
        sessionCost: cost(0.018),
        sessionCurrency: currency,
        sessionCostUsd: cost(0.018),
        sources: {
          executor: {
            promptTokens: 24100,
            completionTokens: 8300,
            totalTokens: 32400,
            reasoningTokens: 5200,
            cacheHitTokens: 76000,
            cacheMissTokens: 9000,
            requestCount: 4,
            sessionCost: cost(0.0124),
            sessionCurrency: currency,
            sessionCostUsd: cost(0.0124),
          },
          planner: {
            promptTokens: 1800,
            completionTokens: 600,
            totalTokens: 2400,
            reasoningTokens: 420,
            cacheHitTokens: 3400,
            cacheMissTokens: 700,
            requestCount: 1,
            sessionCost: cost(0.0011),
            sessionCurrency: currency,
            sessionCostUsd: cost(0.0011),
          },
          subagent: {
            promptTokens: 4200,
            completionTokens: 2100,
            totalTokens: 6300,
            reasoningTokens: 1500,
            cacheHitTokens: 6100,
            cacheMissTokens: 2100,
            requestCount: 2,
            sessionCost: cost(0.0032),
            sessionCurrency: currency,
            sessionCostUsd: cost(0.0032),
          },
          compaction: {
            promptTokens: 2600,
            completionTokens: 700,
            totalTokens: 3300,
            reasoningTokens: 260,
            cacheHitTokens: 1100,
            cacheMissTokens: 900,
            requestCount: 1,
            sessionCost: cost(0.0009),
            sessionCurrency: currency,
            sessionCostUsd: cost(0.0009),
          },
          classifier: {
            promptTokens: 900,
            completionTokens: 120,
            totalTokens: 1020,
            reasoningTokens: 70,
            cacheHitTokens: 300,
            cacheMissTokens: 250,
            requestCount: 1,
            sessionCost: cost(0.0003),
            sessionCurrency: currency,
            sessionCostUsd: cost(0.0003),
          },
          title: {
            promptTokens: 420,
            completionTokens: 80,
            totalTokens: 500,
            reasoningTokens: 20,
            cacheHitTokens: 100,
            cacheMissTokens: 50,
            requestCount: 1,
            sessionCost: cost(0.0001),
            sessionCurrency: currency,
            sessionCostUsd: cost(0.0001),
          },
        },
        mock: true,
        readFiles: [
          { path: "README.md", turn: 2, time: now - 34 * 60 * 1000 },
          { path: "go.mod", turn: 3, time: now - 30 * 60 * 1000 },
          { path: "desktop/file.go", turn: 5, time: now - 13 * 60 * 1000, offset: 0, limit: 180 },
          { path: "internal/event.go", turn: 6, time: now - 4 * 60 * 1000, offset: 120, limit: 80, truncated: true },
        ],
        changedFiles: [
          { path: t("mock.changedFile1Path"), sources: ["session"], gitStatus: "modified", turns: [5, 6], latestPrompt: t("mock.changedFile1Prompt"), latestTime: now - 2 * 60 * 1000 },
          { path: t("mock.changedFile2Path"), sources: ["session"], gitStatus: "added", turns: [6], latestPrompt: t("mock.changedFile2Prompt"), latestTime: now - 60 * 1000 },
        ],
      };
    },
  };
}
