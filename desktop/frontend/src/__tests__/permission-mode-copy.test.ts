// Run: tsx src/__tests__/permission-mode-copy.test.ts

import { en } from "../locales/en";
import { zh } from "../locales/zh";
import { zhTW } from "../locales/zh-TW";

let passed = 0;
let failed = 0;

function ok(condition: boolean, label: string) {
  if (condition) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}\n`);
    failed += 1;
  }
}

console.log("\npermission mode copy");

for (const [locale, messages] of [["en", en], ["zh", zh], ["zh-TW", zhTW]] as const) {
  const yoloCopy = [
    messages["composer.modeFullAccessTitle"],
    messages["composer.accessYoloTitle"],
    messages["composer.accessYoloDesc"],
    messages["composer.accessFullTitle"],
    messages["composer.accessFullDesc"],
    messages["heartbeat.approvalModeYoloTooltip"],
    messages["heartbeat.approvalModeYoloHint"],
  ].join(" ");

  ok(!/skip(?:s)? all|跳过所有|跳過所有/i.test(yoloCopy), `${locale} avoids an absolute bypass claim`);
  ok(/ordinary|普通/.test(yoloCopy), `${locale} describes ordinary-tool auto approval`);
  ok(/deny|拒绝|拒絕/.test(yoloCopy), `${locale} preserves deny boundaries`);
  ok(/trust|信任/.test(yoloCopy), `${locale} preserves fresh-trust prompts`);
}

console.log(`\n${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
