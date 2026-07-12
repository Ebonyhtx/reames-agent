// Run: tsx src/__tests__/dpi-scale.test.ts

import { createZoomWriteQueue, snapZoom } from "../lib/dpiScale";

let passed = 0;
let failed = 0;

function eq(actual: unknown, expected: unknown, label: string) {
  if (JSON.stringify(actual) === JSON.stringify(expected)) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}\n`);
    failed += 1;
  }
}

function deferred() {
  let resolve!: () => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<void>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

console.log("\ndisplay zoom persistence");

eq(snapZoom(0.47), 0.5, "zoom is clamped to the minimum");
eq(snapZoom(2.04), 2, "zoom is clamped to the maximum");
eq(snapZoom(1.024), 1, "zoom snaps to the nearest supported step");
eq(snapZoom(1.026), 1.05, "zoom rounds up across a step midpoint");

{
  const writes: number[] = [];
  const gates = [deferred(), deferred()];
  const queue = createZoomWriteQueue(async (value) => {
    writes.push(value);
    await gates[writes.length - 1].promise;
  });

  const first = queue.enqueue(0.8);
  const second = queue.enqueue(1.1);
  const last = queue.enqueue(1.35);
  eq(writes, [0.8], "only one bridge write runs at a time");

  gates[0].resolve();
  await Promise.resolve();
  await Promise.resolve();
  eq(writes, [0.8, 1.35], "pending slider changes coalesce to the latest value");

  gates[1].resolve();
  eq(await first, 1.35, "first caller observes the final persisted value");
  eq(await second, 1.35, "middle caller observes the final persisted value");
  eq(await last, 1.35, "last caller observes the final persisted value");
}

{
  const writes: number[] = [];
  const queue = createZoomWriteQueue(async (value) => {
    writes.push(value);
    if (writes.length === 1) throw new Error("disk full");
  });
  let message = "";
  try {
    await queue.enqueue(1.2);
  } catch (error) {
    message = String((error as Error).message);
  }
  eq(message, "disk full", "write failures reach the settings surface");
  eq(await queue.enqueue(0.9), 0.9, "queue accepts a fresh value after a failed write");
  eq(writes, [1.2, 0.9], "failed pending state does not leak into the next write");
}

console.log(`\n${passed} passed, ${failed} failed, ${passed + failed} total`);
if (failed > 0) process.exit(1);
