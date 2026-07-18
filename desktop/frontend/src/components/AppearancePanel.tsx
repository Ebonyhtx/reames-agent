import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { AlertTriangle, Check, Download, Loader2, Minus, Plus, RotateCcw, Trash2 } from "lucide-react";

import { app } from "../lib/bridge";
import { useT, type DictKey } from "../lib/i18n";
import {
  THEME_STYLES,
  applyTheme,
  type Theme,
  type ThemeStyle,
} from "../lib/theme";
import { applyThemePack } from "../lib/themePack";
import { TEXT_SIZES, type TextSize } from "../lib/textSize";
import { DEFAULT_ZOOM, MAX_ZOOM, MIN_ZOOM, ZOOM_STEP, zoomToPercent, type ZoomLevel } from "../lib/dpiScale";
import { getAvailableFontFamilies, getAvailableMonoFontFamilies } from "../lib/fontAvailability";
import type { FontFamily, MonoFontFamily } from "../lib/fontFamily";
import type { ThemeExperienceView, ThemeImportResult, ThemePackView } from "../lib/types";

const THEME_STYLE_META: Record<ThemeStyle, { name: string; zh: DictKey; note: DictKey; desc: DictKey }> = {
  graphite: { name: "Graphite", zh: "settings.style.graphite.zh", note: "settings.style.graphite.note", desc: "settings.style.graphite.desc" },
  aurora: { name: "Aurora", zh: "settings.style.aurora.zh", note: "settings.style.aurora.note", desc: "settings.style.aurora.desc" },
  slate: { name: "Slate", zh: "settings.style.slate.zh", note: "settings.style.slate.note", desc: "settings.style.slate.desc" },
  carbon: { name: "Carbon", zh: "settings.style.carbon.zh", note: "settings.style.carbon.note", desc: "settings.style.carbon.desc" },
  nocturne: { name: "Nocturne", zh: "settings.style.nocturne.zh", note: "settings.style.nocturne.note", desc: "settings.style.nocturne.desc" },
  amber: { name: "Amber", zh: "settings.style.amber.zh", note: "settings.style.amber.note", desc: "settings.style.amber.desc" },
};

export interface AppearancePanelProps {
  theme: Theme;
  themeStyle: ThemeStyle;
  textSize: TextSize;
  showDisplayZoom: boolean;
  zoomPct: number;
  zoomRestartRequired: boolean;
  zoomSaving: boolean;
  zoomRestarting: boolean;
  fontFamily: FontFamily;
  monoFontFamily: MonoFontFamily;
  customFontName: string;
  customMonoFontName: string;
  onTheme: (theme: Theme) => void;
  onCommittedThemeStyle: (style: ThemeStyle) => void;
  onTextSize: (size: TextSize) => void;
  onRestartZoom: (zoom: ZoomLevel) => Promise<void>;
  onRestartForZoom: () => Promise<void>;
  onFontFamily: (font: FontFamily) => void;
  onMonoFontFamily: (font: MonoFontFamily) => void;
  onCustomFontNameChange: (name: string) => void;
  onCustomMonoFontNameChange: (name: string) => void;
}

export function AppearancePanel(props: AppearancePanelProps) {
  const t = useT();
  const [experience, setExperience] = useState<ThemeExperienceView | null>(null);
  const [selectedID, setSelectedID] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [pendingReplace, setPendingReplace] = useState<ThemeImportResult | null>(null);
  const [deleteID, setDeleteID] = useState("");
  const latestExperience = useRef<ThemeExperienceView | null>(null);

  const syncVisual = (next: ThemeExperienceView) => {
    latestExperience.current = next;
    setExperience(next);
    const pack = next.packs.find((item) => item.kind !== "base" && item.id === next.effectiveId) ?? null;
    applyThemePack(pack);
    applyTheme(props.theme, props.themeStyle, { persist: false });
  };

  const load = async () => {
    setLoading(true);
    setError("");
    try {
      const next = await app.ThemeExperience();
      syncVisual(next);
      setSelectedID(next.effectiveId || next.appliedThemeId || next.baseStyle);
    } catch (cause) {
      setError(formatError(cause));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
    return () => {
      const current = latestExperience.current;
      if (!current) return;
      const applied = current.packs.find((item) => item.kind !== "base" && item.id === current.appliedThemeId) ?? null;
      applyThemePack(applied);
      applyTheme(props.theme, props.themeStyle, { persist: false });
      if (current.previewThemeId) void app.CancelThemePreview().catch(() => {});
    };
    // Opening/closing the lazy route owns one preview lease. Parent preference
    // changes are projected by their dedicated callbacks, not by remounting it.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const selected = experience?.packs.find((pack) => pack.id === selectedID);
  const appliedID = experience?.appliedThemeId || experience?.baseStyle || props.themeStyle;
  const hasPendingPreview = Boolean(selectedID && selectedID !== appliedID);

  const selectPack = async (pack: ThemePackView) => {
    if (busy || experience?.safeMode) return;
    setSelectedID(pack.id);
    setError("");
    try {
      if (pack.kind === "base") {
        const next = await app.CancelThemePreview();
        latestExperience.current = next;
        applyThemePack(null);
        applyTheme(props.theme, pack.id as ThemeStyle, { persist: false });
      } else {
        syncVisual(await app.PreviewThemePack(pack.id));
      }
    } catch (cause) {
      setError(formatError(cause));
    }
  };

  const cancelPreview = async () => {
    setBusy(true);
    setError("");
    try {
      const next = await app.CancelThemePreview();
      syncVisual(next);
      setSelectedID(next.appliedThemeId || next.baseStyle);
    } catch (cause) {
      setError(formatError(cause));
    } finally {
      setBusy(false);
    }
  };

  const applySelected = async () => {
    if (!selected) return;
    setBusy(true);
    setError("");
    try {
      let next: ThemeExperienceView;
      if (selected.kind === "base") {
        next = await app.ApplyThemePack("");
        await app.SetDesktopAppearance(props.theme, selected.id);
        props.onCommittedThemeStyle(selected.id as ThemeStyle);
        next = await app.ThemeExperience();
      } else {
        next = await app.ApplyThemePack(selected.id);
      }
      syncVisual(next);
      setSelectedID(next.appliedThemeId || next.baseStyle);
    } catch (cause) {
      setError(formatError(cause));
    } finally {
      setBusy(false);
    }
  };

  const importTheme = async () => {
    setBusy(true);
    setError("");
    try {
      const result = await app.ImportThemePack();
      if (result.canceled) return;
      if (result.needsReplace && result.pendingId) {
        setPendingReplace(result);
        return;
      }
      await load();
      setSelectedID(result.pack.id);
    } catch (cause) {
      setError(formatError(cause));
    } finally {
      setBusy(false);
    }
  };

  const confirmReplace = async () => {
    const pendingID = pendingReplace?.pendingId;
    if (!pendingID) return;
    setBusy(true);
    setError("");
    try {
      const result = await app.ConfirmThemePackImport(pendingID);
      setPendingReplace(null);
      await load();
      setSelectedID(result.pack.id);
    } catch (cause) {
      setError(formatError(cause));
    } finally {
      setBusy(false);
    }
  };

  const cancelReplace = () => {
    const pendingID = pendingReplace?.pendingId;
    if (pendingID) void app.CancelThemePackImport(pendingID);
    setPendingReplace(null);
  };

  const deleteTheme = async (id: string) => {
    setBusy(true);
    setError("");
    try {
      const next = await app.DeleteThemePack(id);
      syncVisual(next);
      setSelectedID(next.appliedThemeId || next.baseStyle);
      setDeleteID("");
    } catch (cause) {
      setError(formatError(cause));
    } finally {
      setBusy(false);
    }
  };

  const themeOptions: Theme[] = ["auto", "light", "dark"];
  const availableFontFamilies = useMemo(() => getAvailableFontFamilies(props.fontFamily), [props.fontFamily]);
  const availableMonoFontFamilies = useMemo(() => getAvailableMonoFontFamilies(props.monoFontFamily), [props.monoFontFamily]);
  const zoomMinPct = zoomToPercent(MIN_ZOOM);
  const zoomMaxPct = zoomToPercent(MAX_ZOOM);
  const zoomStepPct = Math.round(ZOOM_STEP * 100);
  const zoomProgressPct = Math.min(100, Math.max(0, ((props.zoomPct - zoomMinPct) / (zoomMaxPct - zoomMinPct)) * 100));

  return (
    <div className="appearance-panel">
      <AppearanceSection title={t("settings.appearance")}>
        <AppearanceField label={t("settings.theme")}>
          <div className="set-seg">
            {themeOptions.map((option) => (
              <button
                key={option}
                type="button"
                className={`set-seg__btn${props.theme === option ? " set-seg__btn--on" : ""}`}
                onClick={() => {
                  props.onTheme(option);
                  const base = selected?.kind === "base" ? selected.id as ThemeStyle : props.themeStyle;
                  applyTheme(option, base, { persist: false });
                }}
              >
                {themeName(option, t)}
              </button>
            ))}
          </div>
        </AppearanceField>
      </AppearanceSection>

      <AppearanceSection
        title={t("settings.themeStyle")}
        action={(
          <button type="button" className="btn btn--small" disabled={busy || experience?.safeMode} onClick={() => void importTheme()}>
            <Download size={14} aria-hidden="true" /> {t("common.add")}
          </button>
        )}
      >
        {loading ? (
          <div className="empty" role="status"><Loader2 className="spin" size={16} /> {t("common.loading")}</div>
        ) : (
          <>
            {experience?.safeMode && <div className="banner banner--warning" role="status">{t("settings.themeStyle")}: Graphite</div>}
            {experience?.warnings.map((warning) => <div key={warning} className="banner banner--warning">{warning}</div>)}
            {error && <div className="banner banner--error" role="alert">{error}</div>}
            <div className="theme-card-grid theme-pack-grid" role="radiogroup" aria-label={t("settings.themeStyle")}>
              {experience?.packs.map((pack) => {
                const isSelected = selectedID === pack.id;
                const meta = pack.kind === "base" && THEME_STYLES.includes(pack.id as ThemeStyle) ? THEME_STYLE_META[pack.id as ThemeStyle] : null;
                return (
                  <button
                    key={pack.id}
                    type="button"
                    role="radio"
                    aria-checked={isSelected}
                    className={`theme-card theme-pack-card${isSelected ? " theme-card--on" : ""}`}
                    disabled={busy || experience?.safeMode}
                    onClick={() => void selectPack(pack)}
                  >
                    {pack.scenes.home?.imageUrl && <span className="theme-pack-card__scene" style={{ backgroundImage: `url("${pack.scenes.home.imageUrl}")` }} aria-hidden="true" />}
                    <span className="theme-card__head">
                      <span className="theme-card__name">{pack.name}{meta && <span className="theme-card__zh"> {t(meta.zh)}</span>}</span>
                      <span className="theme-card__tag">{pack.kind === "base" ? t(meta?.note ?? "settings.themeStyle") : pack.version || pack.license}</span>
                    </span>
                    <span className="theme-card__swatches" data-theme-style-card={pack.baseStyle}>
                      <span className="theme-card__swatch theme-card__swatch--bg" />
                      <span className="theme-card__swatch theme-card__swatch--surface" />
                      <span className="theme-card__swatch theme-card__swatch--accent" style={pack.tokens.dark.accent ? { background: pack.tokens.dark.accent } : undefined} />
                    </span>
                    <span className="theme-card__desc">{pack.description || (meta ? t(meta.desc) : pack.provenance.source)}</span>
                    {pack.kind !== "base" && <span className="theme-pack-card__meta">{pack.author ? `${pack.author} · ` : ""}{pack.license}</span>}
                    <span className="theme-card__check" aria-hidden="true"><Check size={13} strokeWidth={3} /></span>
                  </button>
                );
              })}
            </div>
            {selected && (
              <div className="theme-pack-selection" aria-live="polite">
                <div>
                  <strong>{selected.name}</strong>
                  <span>{selected.kind !== "base" ? `${selected.provenance.source} · ${selected.license}` : t("settings.themeStyle")}</span>
                </div>
                <div className="theme-pack-selection__actions">
                  {selected.kind === "user" && (
                    <button type="button" className="btn btn--small btn--danger" disabled={busy} onClick={() => setDeleteID(selected.id)}>
                      <Trash2 size={13} aria-hidden="true" /> {t("common.delete")}
                    </button>
                  )}
                  <button type="button" className="btn btn--small" disabled={busy || !hasPendingPreview} onClick={() => void cancelPreview()}>{t("common.cancel")}</button>
                  <button type="button" className="btn btn--small btn--primary" disabled={busy || !hasPendingPreview} onClick={() => void applySelected()}>{t("common.save")}</button>
                </div>
              </div>
            )}
            {selected && selected.contrastWarnings.length > 0 && (
              <div className="theme-contrast-warnings" role="status">
                <strong><AlertTriangle size={14} aria-hidden="true" /> {t("settings.theme")}</strong>
                {selected.contrastWarnings.map((warning) => (
                  <span key={`${warning.mode}-${warning.pair}`}>{warning.mode}: {warning.pair} {warning.ratio}:1 / {warning.minimum}:1</span>
                ))}
              </div>
            )}
            {pendingReplace && (
              <div className="inline-confirm" role="alertdialog" aria-label={`${t("common.save")} ${pendingReplace.pack.name}?`}>
                <span>{t("common.save")} {pendingReplace.pack.name}?</span>
                <div>
                  <button type="button" className="btn btn--small" onClick={cancelReplace}>{t("common.cancel")}</button>
                  <button type="button" className="btn btn--small btn--danger" disabled={busy} onClick={() => void confirmReplace()}>{t("common.save")}</button>
                </div>
              </div>
            )}
            {deleteID && (
              <div className="inline-confirm" role="alertdialog" aria-label={`${t("common.delete")} ${experience?.packs.find((pack) => pack.id === deleteID)?.name ?? deleteID}?`}>
                <span>{t("common.delete")} {experience?.packs.find((pack) => pack.id === deleteID)?.name ?? deleteID}?</span>
                <div>
                  <button type="button" className="btn btn--small" onClick={() => setDeleteID("")}>{t("common.cancel")}</button>
                  <button type="button" className="btn btn--small btn--danger" disabled={busy} onClick={() => void deleteTheme(deleteID)}>{t("common.delete")}</button>
                </div>
              </div>
            )}
          </>
        )}
      </AppearanceSection>

      <AppearanceSection title={t("settings.textSize")}>
        <AppearanceField label={t("settings.textSize")}>
          <div className="set-seg">
            {TEXT_SIZES.map((size) => <button key={size} type="button" className={`set-seg__btn${props.textSize === size ? " set-seg__btn--on" : ""}`} onClick={() => props.onTextSize(size)}>{textSizeName(size, t)}</button>)}
          </div>
        </AppearanceField>
        {props.showDisplayZoom && (
          <AppearanceField label={t("settings.displayZoom")}>
            <div className="zoom-slider-wrap">
              <div className="zoom-slider__head">
                <div className="zoom-slider__value">{props.zoomPct}%</div>
                <div className="zoom-stepper">
                  <button type="button" className="zoom-stepper__btn" aria-label={t("settings.displayZoomDecrease")} disabled={props.zoomPct <= zoomMinPct} onClick={() => void props.onRestartZoom((props.zoomPct - zoomStepPct) / 100)}><Minus size={13} /></button>
                  <button type="button" className="zoom-stepper__reset" aria-label={t("settings.displayZoomReset")} disabled={props.zoomPct === zoomToPercent(DEFAULT_ZOOM)} onClick={() => void props.onRestartZoom(DEFAULT_ZOOM)}><RotateCcw size={12} /><span>100%</span></button>
                  <button type="button" className="zoom-stepper__btn" aria-label={t("settings.displayZoomIncrease")} disabled={props.zoomPct >= zoomMaxPct} onClick={() => void props.onRestartZoom((props.zoomPct + zoomStepPct) / 100)}><Plus size={13} /></button>
                </div>
              </div>
              <div className="zoom-slider-row">
                <span className="zoom-slider__label">{zoomMinPct}%</span>
                <div className="slider-track">
                  <div className="slider-track__bg" />
                  <div className="slider-track__fill" style={{ width: `calc(${zoomProgressPct}% + 15px)` }} />
                  <div className="slider-thumb" style={{ left: `${zoomProgressPct}%` }} />
                  <input aria-label={t("settings.displayZoom")} aria-valuetext={`${props.zoomPct}%`} type="range" min={zoomMinPct} max={zoomMaxPct} step={zoomStepPct} value={props.zoomPct} onChange={(event) => void props.onRestartZoom(Number(event.target.value) / 100)} />
                </div>
                <span className="zoom-slider__label">{zoomMaxPct}%</span>
              </div>
              <div className="zoom-slider__status" role="status" aria-live="polite">
                {props.zoomSaving ? t("settings.displayZoomSaving") : props.zoomRestartRequired ? <><span>{t("settings.displayZoomPending", { zoom: props.zoomPct })}</span><button type="button" className="btn btn--small" disabled={props.zoomRestarting} onClick={() => void props.onRestartForZoom()}>{props.zoomRestarting ? t("settings.displayZoomRestarting") : t("settings.displayZoomRestart")}</button></> : t("settings.displayZoomApplied")}
              </div>
            </div>
          </AppearanceField>
        )}
        <AppearanceField label={t("settings.fontFamily")}>
          <div className="set-seg">{availableFontFamilies.map((font) => <button key={font} type="button" className={`set-seg__btn${props.fontFamily === font ? " set-seg__btn--on" : ""}`} onClick={() => props.onFontFamily(font)}>{fontFamilyName(font, t)}</button>)}</div>
        </AppearanceField>
        {props.fontFamily === "custom" && <AppearanceField label={t("settings.fontFamilyCustomName")}><textarea className="mem-input" rows={2} value={props.customFontName} placeholder={t("settings.fontFamilyCustomPlaceholder")} onChange={(event) => props.onCustomFontNameChange(event.target.value)} /></AppearanceField>}
        <AppearanceField label={t("settings.monoFontFamily")}>
          <div className="set-seg">{availableMonoFontFamilies.map((font) => <button key={font} type="button" className={`set-seg__btn${props.monoFontFamily === font ? " set-seg__btn--on" : ""}`} onClick={() => props.onMonoFontFamily(font)}>{monoFontFamilyName(font, t)}</button>)}</div>
        </AppearanceField>
        {props.monoFontFamily === "custom" && <AppearanceField label={t("settings.monoFontFamilyCustomName")}><textarea className="mem-input" rows={2} value={props.customMonoFontName} placeholder={t("settings.monoFontFamilyCustomPlaceholder")} onChange={(event) => props.onCustomMonoFontNameChange(event.target.value)} /></AppearanceField>}
      </AppearanceSection>
    </div>
  );
}

function AppearanceSection({ title, description, action, children }: { title: string; description?: string; action?: ReactNode; children: ReactNode }) {
  return <section className="settings-section"><header className="settings-section__head"><div><h3>{title}</h3>{description && <p>{description}</p>}</div>{action}</header><div className="settings-section__body">{children}</div></section>;
}

function AppearanceField({ label, children }: { label: string; children: ReactNode }) {
  return <div className="settings-field"><div className="settings-field__label"><span>{label}</span></div><div className="settings-field__control">{children}</div></div>;
}

function themeName(theme: Theme, t: ReturnType<typeof useT>) {
  return theme === "auto" ? t("settings.themeAuto") : theme === "light" ? t("settings.themeLight") : t("settings.themeDark");
}

function textSizeName(size: TextSize, t: ReturnType<typeof useT>) {
  const keys: Record<TextSize, DictKey> = { small: "settings.textSizeSmall", default: "settings.textSizeDefault", large: "settings.textSizeLarge", xlarge: "settings.textSizeXLarge", xxlarge: "settings.textSizeXXLarge" };
  return t(keys[size]);
}

function fontFamilyName(font: FontFamily, t: ReturnType<typeof useT>) {
  const keys: Record<FontFamily, DictKey> = { system: "settings.fontFamilySystem", yahei: "settings.fontFamilyYaHei", pingfang: "settings.fontFamilyPingFang", noto: "settings.fontFamilyNoto", custom: "settings.fontFamilyCustom" };
  return t(keys[font]);
}

function monoFontFamilyName(font: MonoFontFamily, t: ReturnType<typeof useT>) {
  const keys: Record<MonoFontFamily, DictKey> = { system: "settings.monoFontFamilySystem", cascadia: "settings.monoFontFamilyCascadia", jetbrains: "settings.monoFontFamilyJetBrains", sfmono: "settings.monoFontFamilySFMono", custom: "settings.monoFontFamilyCustom" };
  return t(keys[font]);
}

function formatError(cause: unknown): string {
  return cause instanceof Error ? cause.message : String(cause);
}
