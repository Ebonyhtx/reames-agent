// Run: tsx src/__tests__/theme-contrast.test.ts

import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

type RGB = { r: number; g: number; b: number };
type Tokens = Record<string, string>;

const testDir = dirname(fileURLToPath(import.meta.url));
const css = readFileSync(resolve(testDir, "../styles.css"), "utf8");
const styles = ["graphite", "aurora", "slate", "carbon", "nocturne", "amber"] as const;

let passed = 0;
let failed = 0;

function ok(value: boolean, label: string) {
  process.stdout.write(`  ${value ? "PASS" : "FAIL"}  ${label}\n`);
  if (value) passed += 1;
  else failed += 1;
}

function selectorBlocks(selector: string): Array<{ start: number; body: string }> {
  const marker = `${selector} {`;
  const blocks: Array<{ start: number; body: string }> = [];
  let cursor = 0;
  while (cursor < css.length) {
    const start = css.indexOf(marker, cursor);
    if (start < 0) break;
    const bodyStart = start + marker.length;
    let depth = 1;
    for (let i = bodyStart; i < css.length; i += 1) {
      if (css[i] === "{") depth += 1;
      else if (css[i] === "}") depth -= 1;
      if (depth === 0) {
        blocks.push({ start, body: css.slice(bodyStart, i) });
        cursor = i + 1;
        break;
      }
    }
  }
  if (blocks.length === 0) throw new Error(`missing CSS selector: ${selector}`);
  return blocks;
}

function tokensFrom(block: string): Tokens {
  const tokens: Tokens = {};
  for (const match of block.matchAll(/--([a-z0-9-]+)\s*:\s*([^;]+);/gi)) {
    tokens[match[1]] = match[2].trim();
  }
  return tokens;
}

function cascadeTokens(selectors: string[]): Tokens {
  const blocks = selectors
    .flatMap((selector) => selectorBlocks(selector).map((block) => ({
      ...block,
      specificity: (selector.match(/[:.\[]/g) ?? []).length,
    })))
    .sort((left, right) => left.start - right.start);
  const resolved: Record<string, { value: string; specificity: number }> = {};
  for (const block of blocks) {
    for (const [name, value] of Object.entries(tokensFrom(block.body))) {
      const current = resolved[name];
      if (!current || block.specificity >= current.specificity) {
        resolved[name] = { value, specificity: block.specificity };
      }
    }
  }
  return Object.fromEntries(Object.entries(resolved).map(([name, entry]) => [name, entry.value]));
}

function resolveToken(tokens: Tokens, name: string, seen = new Set<string>()): string {
  if (seen.has(name)) throw new Error(`cyclic CSS token: --${name}`);
  seen.add(name);
  const raw = tokens[name];
  if (!raw) throw new Error(`missing CSS token: --${name}`);
  const direct = /^var\(--([a-z0-9-]+)\)$/i.exec(raw);
  return direct ? resolveToken(tokens, direct[1], seen) : raw;
}

function parseHex(value: string): RGB {
  const match = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(value);
  if (!match) throw new Error(`contrast token must resolve to a solid hex color, got ${value}`);
  const hex = match[1].length === 3 ? match[1].split("").map((part) => part + part).join("") : match[1];
  return {
    r: Number.parseInt(hex.slice(0, 2), 16),
    g: Number.parseInt(hex.slice(2, 4), 16),
    b: Number.parseInt(hex.slice(4, 6), 16),
  };
}

function luminance(color: RGB): number {
  const [r, g, b] = [color.r, color.g, color.b].map((channel) => {
    const value = channel / 255;
    return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function contrast(tokens: Tokens, foreground: string, background: string): number {
  const fg = luminance(parseHex(resolveToken(tokens, foreground)));
  const bg = luminance(parseHex(resolveToken(tokens, background)));
  return (Math.max(fg, bg) + 0.05) / (Math.min(fg, bg) + 0.05);
}

function expectContrast(tokens: Tokens, foreground: string, background: string, minimum: number, label: string) {
  const ratio = contrast(tokens, foreground, background);
  ok(ratio + 1e-9 >= minimum, `${label}: ${ratio.toFixed(2)}:1 >= ${minimum}:1`);
}

function paletteTokens(style: (typeof styles)[number], scheme: "dark" | "light" | "auto-light"): Tokens {
  const styleSelector = `:root[data-theme-style="${style}"]`;
  const focusSelector = ':root[data-theme-style]:not([data-theme-style=""])';
  if (scheme === "dark") {
    return cascadeTokens([":root", styleSelector, focusSelector]);
  }
  if (scheme === "light") {
    return cascadeTokens([
      ":root",
      ':root[data-theme="light"]',
      styleSelector,
      `:root[data-theme="light"][data-theme-style="${style}"]`,
      ':root[data-theme="light"][data-theme-style]',
      focusSelector,
    ]);
  }
  return cascadeTokens([
    ":root",
    ":root:not([data-theme])",
    styleSelector,
    `:root[data-theme-style="${style}"]:not([data-theme])`,
    ":root[data-theme-style]:not([data-theme])",
    focusSelector,
  ]);
}

function creationPaletteTokens(
  style: (typeof styles)[number],
  scheme: "dark" | "light" | "auto-light",
): Tokens {
  const inherited = paletteTokens(style, scheme);
  const selectors = scheme === "dark"
    ? [
        ':root[data-theme="dark"][data-theme-style] .app--creation',
        `:root[data-theme="dark"][data-theme-style="${style}"] .app--creation`,
      ]
    : scheme === "light"
      ? [
          ':root[data-theme="light"][data-theme-style] .app--creation',
          `:root[data-theme="light"][data-theme-style="${style}"] .app--creation`,
        ]
      : [
          ':root[data-theme-style]:not([data-theme]) .app--creation',
          `:root[data-theme-style="${style}"]:not([data-theme]) .app--creation`,
        ];
  return { ...inherited, ...cascadeTokens(selectors) };
}

function checkPalette(tokens: Tokens, prefix: string) {
  for (const token of ["fg", "fg-dim", "fg-faint", "accent", "ok", "warn", "err", "add-fg", "del-fg"]) {
    expectContrast(tokens, token, "bg", 4.5, `${prefix} --${token} text on --bg`);
  }
  expectContrast(tokens, "control-primary-fg", "control-primary-bg", 4.5, `${prefix} primary control text`);
  expectContrast(tokens, "accent", "bg-elev", 3, `${prefix} focus indicator on elevated surface`);
  ok(
    tokens["focus-ring"] === "0 0 0 2px var(--bg), 0 0 0 4px var(--accent)",
    `${prefix} uses the opaque two-layer focus ring`,
  );
}

console.log("\ntheme contrast contract");

for (const style of styles) {
  for (const scheme of ["dark", "light"] as const) {
    checkPalette(paletteTokens(style, scheme), `${style}/${scheme}/classic`);
    checkPalette(creationPaletteTokens(style, scheme), `${style}/${scheme}/creation`);
  }
}

for (const style of styles) {
  const explicit = paletteTokens(style, "light");
  const automatic = paletteTokens(style, "auto-light");
  const critical = ["bg", "bg-elev", "fg", "fg-dim", "fg-faint", "accent", "ok", "warn", "err", "control-primary-bg", "control-primary-fg"];
  const mismatches = critical.filter((token) => resolveToken(explicit, token) !== resolveToken(automatic, token));
  ok(
    mismatches.length === 0,
    `${style} auto-light accessibility tokens match explicit light${mismatches.length > 0 ? ` (${mismatches.map((token) => `${token}:${resolveToken(explicit, token)}/${resolveToken(automatic, token)}`).join(", ")})` : ""}`,
  );

  const explicitCreation = creationPaletteTokens(style, "light");
  const automaticCreation = creationPaletteTokens(style, "auto-light");
  const creationMismatches = critical.filter(
    (token) => resolveToken(explicitCreation, token) !== resolveToken(automaticCreation, token),
  );
  ok(
    creationMismatches.length === 0,
    `${style} creation auto-light accessibility tokens match explicit light${creationMismatches.length > 0 ? ` (${creationMismatches.map((token) => `${token}:${resolveToken(explicitCreation, token)}/${resolveToken(automaticCreation, token)}`).join(", ")})` : ""}`,
  );
}

ok(
  !/outline\s*:[^;]*var\(--focus-ring\)/.test(css),
  "focus ring shadow token is never used as an outline color",
);
ok(
  /@media\s*\(forced-colors:\s*active\)[\s\S]*outline\s*:\s*2px solid Highlight !important;[\s\S]*box-shadow\s*:\s*none !important;/.test(css),
  "forced-colors mode overrides component focus shadows with a system-color ring",
);

for (const selector of [
  ':root[data-theme="dark"][data-theme-style] .app--creation',
  ':root[data-theme="light"][data-theme-style] .app--creation',
  ':root[data-theme-style]:not([data-theme]) .app--creation',
]) {
  ok(
    selectorBlocks(selector).some(
      (block) => tokensFrom(block.body)["focus-ring"] === "0 0 0 2px var(--bg), 0 0 0 4px var(--accent)",
    ),
    `${selector} recomputes the focus ring against its local canvas`,
  );
}

console.log(`\n${passed} passed, ${failed} failed, ${passed + failed} total`);
if (failed > 0) process.exit(1);
