// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0
//
// Fails when emoji code points appear in src/ or index.html (icons come from lucide-react via <Icon/>).
// Allowed: the documented compact symbols (◆ ⧉ ⇄ ▲ ▼ ~ ≈ → ← ↔ ▸ ▾ × ·) and ©.
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';

const ALLOW = new Set([...'◆⧉⇄▲▼~≈→←↔▸▾×·©']);
const EXTRA = /[✓✔✗✘★☆✂⛁⛔⭐⏵-⏺▶◀\u{1F300}-\u{1FAFF}️]/u;
const BAD = /\p{Extended_Pictographic}/u;
const roots = process.argv.slice(2).length ? process.argv.slice(2) : ['src', 'index.html'];
const files = [];
const walk = (p) => {
  const st = statSync(p);
  if (st.isDirectory()) for (const f of readdirSync(p)) walk(join(p, f));
  else if (/\.(tsx?|css|html|json|md|xml|yaml|ya?ml)$/.test(p)) files.push(p);
};
roots.forEach(walk);
let n = 0;
for (const f of files) {
  readFileSync(f, 'utf8')
    .split('\n')
    .forEach((line, i) => {
      for (const ch of line) {
        if (ALLOW.has(ch)) continue;
        if (BAD.test(ch) || EXTRA.test(ch)) {
          n++;
          console.log(`${f}:${i + 1}: emoji ${JSON.stringify(ch)} U+${ch.codePointAt(0).toString(16).toUpperCase()}  ${line.trim().slice(0, 100)}`);
          break;
        }
      }
    });
}
if (n) {
  console.error(`\n${n} line(s) with emoji. Use <Icon name="…"/> instead.`);
  process.exit(1);
}
console.log(`lint:emoji ok (${files.length} files)`);
