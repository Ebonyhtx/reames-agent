import { app } from "./bridge";

export async function restoreStartupTheme(): Promise<void> {
  const active = await app.ActiveThemePack();
  const { restoreActiveTheme } = await import("./themePackRoute");
  restoreActiveTheme(active);
  if (active.warning) console.warn(active.warning);
}

void restoreStartupTheme().catch((error) => console.warn("active theme restore failed", error));
