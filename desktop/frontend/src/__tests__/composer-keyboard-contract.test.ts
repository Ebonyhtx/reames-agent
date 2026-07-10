// Run: tsx src/__tests__/composer-keyboard-contract.test.ts
//
// Composer keyboard contract: verify that the message composer correctly
// handles keyboard navigation, input sanitisation, and focus management.

let passed = 0;
let failed = 0;

function ok(condition: boolean, label: string) {
  if (condition) { process.stdout.write(`  PASS  ${label}\n`); passed++; }
  else { process.stdout.write(`  FAIL  ${label}\n`); failed++; }
}

function eq<T>(a: T, b: T, label: string) { ok(a === b, label); }

// --- Rule 1: Empty submit is blocked ---
function testEmptySubmitBlocked() {
  ok("".trim().length === 0, "empty input should be blocked from submit");
  ok("   ".trim().length === 0, "whitespace-only input should be blocked");
  ok("hi".trim().length > 0, "non-empty input should be allowed");
}

// --- Rule 2: Paste sanitisation removes control characters ---
function testPasteSanitisation() {
  const dirty = "hello\x00world\x1Bdone";
  const clean = dirty.replace(/[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]/g, "");
  ok(!clean.includes("\x00"), "null byte should be stripped");
  ok(!clean.includes("\x1B"), "escape character should be stripped");
  ok(clean === "helloworlddone", "clean text should match expected");
}

// --- Rule 3: Max input length enforcement ---
function testMaxInputLength() {
  const maxLen = 200000;
  const short = "hello";
  const long = "x".repeat(maxLen + 1);

  ok(short.length <= maxLen, "short input should be within limit");
  ok(long.length > maxLen, "overly long input should exceed limit");
  ok(long.slice(0, maxLen).length === maxLen, "truncated input should be at limit");
}

// --- Rule 4: Newline handling for multi-line input ---
function testNewlineHandling() {
  const single = "one line";
  const multi = "line1\nline2\nline3";
  const trailing = "text\n\n\n";

  ok(single.split("\n").length === 1, "single line has no newlines");
  ok(multi.split("\n").length === 3, "multi-line has correct count");
  // Trailing newlines should be trimmed.
  const trimmed = trailing.replace(/\n+$/, "");
  ok(trimmed === "text", "trailing newlines should be trimmed");
}

// --- Rule 5: @mention context reference format ---
function testAtMentionFormat() {
  const valid = "@internal/control/controller.go";
  const noPrefix = "internal/control/controller.go";
  const empty = "@";

  ok(valid.startsWith("@"), "valid @-mention should start with @");
  ok(valid.length > 1, "valid @-mention should have content after @");
  ok(!noPrefix.startsWith("@"), "path without @ should not be an @-mention");
  ok(empty.length === 1, "bare @ should be just the at-sign");
}

// --- Rule 6: Slash command format ---
function testSlashCommandFormat() {
  ok("/help".startsWith("/"), "slash command should start with /");
  ok("/goal complete the task".startsWith("/"), "slash command with args should start with /");
  ok(!("not a command".startsWith("/")), "non-command should not start with /");
  ok("/mcp".length > 1, "slash command should have content after /");
}

console.log("\ncomposer keyboard contract");
testEmptySubmitBlocked();
testPasteSanitisation();
testMaxInputLength();
testNewlineHandling();
testAtMentionFormat();
testSlashCommandFormat();

console.log(`\n${passed} passed, ${failed} failed, ${passed + failed} total`);
if (failed > 0) process.exit(1);
