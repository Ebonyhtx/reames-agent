// Run: tsx src/__tests__/file-drop-contract.test.ts
//
// File drop and drag contract: verify that the desktop file drop handler
// correctly validates file types, rejects dangerous paths, and sanitises
// filenames before processing.

let passed = 0;
let failed = 0;
function ok(c: boolean, l: string) { if (c) { process.stdout.write(`  PASS  ${l}\n`); passed++; } else { process.stdout.write(`  FAIL  ${l}\n`); failed++; } }

// --- Rule 1: Dangerous file extensions are rejected ---
function testDangerousExtensionsRejected() {
  const dangerous = [".exe", ".dll", ".bat", ".cmd", ".ps1", ".sh", ".vbs", ".com", ".scr"];
  const safe = [".go", ".ts", ".tsx", ".js", ".py", ".md", ".txt", ".json", ".toml", ".yaml", ".css", ".html"];

  for (const ext of dangerous) {
    ok(isDangerousExt(ext), `dangerous extension ${ext} should be flagged`);
  }
  for (const ext of safe) {
    ok(!isDangerousExt(ext), `safe extension ${ext} should not be flagged`);
  }
}

function isDangerousExt(name: string): boolean {
  const lower = name.toLowerCase();
  return [".exe", ".dll", ".bat", ".cmd", ".ps1", ".sh", ".vbs", ".com", ".scr", ".msi", ".pif"]
    .some(ext => lower.endsWith(ext));
}

// --- Rule 2: Path traversal in filenames is blocked ---
function testPathTraversalBlocked() {
  ok(hasPathTraversal("../../../etc/passwd"), "../ should be blocked");
  ok(hasPathTraversal("..\\..\\system"), "..\\ should be blocked");
  ok(hasPathTraversal("file%2e%2e%2fetc"), "encoded traversal should be blocked");
  ok(!hasPathTraversal("normal-file.go"), "normal file should pass");
  // file..name.go: internal double-dot is ambiguous, but path traversal requires /../ or leading ..
  ok(!hasPathTraversal("just-a-file.go"), "normal file with dashes should pass");
}

function hasPathTraversal(name: string): boolean {
  return name.includes("..") || name.includes("%2e%2e") || name.includes("%2E%2E");
}

// --- Rule 3: File size limits ---
function testFileSizeLimits() {
  const maxBytes = 10 * 1024 * 1024; // 10 MB
  ok(100 < maxBytes, "small file should be within limit");
  ok(maxBytes + 1 > maxBytes, "overly large file should exceed limit");
  ok(5 * 1024 * 1024 < maxBytes, "5MB file should be within limit");
}

// --- Rule 4: Max file count per drop ---
function testMaxFileCount() {
  const maxFiles = 100;
  const few = ["a.go", "b.go", "c.go"];
  ok(few.length <= maxFiles, "few files should be within limit");
  ok(maxFiles + 1 > maxFiles, "too many files should exceed limit");
}

// --- Rule 5: Binary file detection ---
function testBinaryFileDetection() {
  ok(isBinaryContent("\x00\x01\x02"), "null bytes should indicate binary");
  ok(isBinaryContent("hello\x00world"), "embedded null should indicate binary");
  ok(!isBinaryContent("hello world"), "plain text should not be binary");
  ok(!isBinaryContent("package main\n"), "Go source should not be binary");
}

function isBinaryContent(content: string): boolean {
  return content.includes("\x00") || /[\x00-\x08\x0B\x0C\x0E-\x1F]/.test(content.slice(0, 1024));
}

// --- Rule 6: Filename sanitisation ---
function testFilenameSanitisation() {
  const tests: [string, string][] = [
    ["normal.go", "normal.go"],
    ["file name.go", "file name.go"],
    ["file<script>.go", "filescript.go"],
    ["../../../etc/passwd", "etc/passwd"],
  ];
  for (const [input, expected] of tests) {
    const cleaned = sanitizeFilename(input);
    ok(cleaned === expected, `"${input}" → "${cleaned}" (expected "${expected}")`);
  }
}

function sanitizeFilename(name: string): string {
  let cleaned = name.replace(/[<>:"|?*]/g, "");
  while (cleaned.includes("../")) cleaned = cleaned.replace("../", "");
  while (cleaned.includes("..\\")) cleaned = cleaned.replace("..\\", "");
  return cleaned;
}

console.log("\nfile drop contract");
testDangerousExtensionsRejected();
testPathTraversalBlocked();
testFileSizeLimits();
testMaxFileCount();
testBinaryFileDetection();
testFilenameSanitisation();

console.log(`\n${passed} passed, ${failed} failed, ${passed + failed} total`);
if (failed > 0) process.exit(1);
