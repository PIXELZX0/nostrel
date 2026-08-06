// Checks the panel translations. Run from the repository root:
//
//   node web/i18n_test.mjs
//
// It renders both pages through i18n.js with lang=en and fails on any Korean
// left behind, then checks that every key t() is called with — including the
// settingFields labels, which reach it as a variable — has an English entry.
import fs from "node:fs";
import vm from "node:vm";

const src = fs.readFileSync("web/static/i18n.js", "utf8");

// minimal DOM: only what i18n.js touches
function makeDoc(html) {
  const nodes = [];           // {raw} text nodes, in document order
  const attrs = [];           // {el} placeholder/title holders
  // strip <svg> blocks and script/style, then split into tags and text
  const body = html.replace(/<svg[\s\S]*?<\/svg>/g, "").replace(/<script[\s\S]*?<\/script>/g, "");
  const unesc = (x) => x.replace(/&lt;/g, "<").replace(/&gt;/g, ">").replace(/&amp;/g, "&");
  for (const m of body.matchAll(/>([^<>]+)</g)) nodes.push({ nodeValue: unesc(m[1]) });
  for (const m of body.matchAll(/(placeholder|title)="([^"]*)"/g)) attrs.push({ name: m[1], value: m[2] });

  const el = {
    getAttribute(n) { const a = attrs.find((x) => x.name === n && !x.done); return a ? a.value : null; },
    setAttribute(n, v) { const a = attrs.find((x) => x.name === n && !x.done); if (a) { a.value = v; a.done = true; } },
  };
  const qsa = (sel) => {
    if (sel === "select.lang") return [];
    return attrs.map((a) => ({
      getAttribute: (n) => (n === a.name ? a.value : null),
      setAttribute: (n, v) => { if (n === a.name) a.value = v; },
    }));
  };
  return {
    nodes, attrs,
    documentElement: {},
    body: { querySelectorAll: qsa },
    createTreeWalker() {
      let i = 0;
      return { nextNode: () => (i < nodes.length ? nodes[i++] : null) };
    },
    querySelectorAll(sel) {
      if (sel === "select.lang") return [];
      // one proxy element per attribute occurrence
      return attrs.map((a) => ({
        getAttribute: (n) => (n === a.name ? a.value : null),
        setAttribute: (n, v) => { if (n === a.name) a.value = v; },
      }));
    },
  };
}

let bad = 0;
for (const page of ["web/index.html", "web/admin.html"]) {
  const document = makeDoc(fs.readFileSync(page, "utf8"));
  const ctx = {
    document,
    navigator: { language: "en-US" },
    localStorage: { getItem: () => "en", setItem() {} },
    location: { reload() {} },
    NodeFilter: { SHOW_TEXT: 4 },
    console,
  };
  vm.createContext(ctx);
  vm.runInContext(src, ctx, { filename: "i18n.js" });

  const leftover = [
    // the language options are meant to stay in their own language
    ...document.nodes.filter((n) => /[가-힣]/.test(n.nodeValue) && n.nodeValue.trim() !== "한국어").map((n) => "text: " + n.nodeValue.trim()),
    ...document.attrs.filter((a) => /[가-힣]/.test(a.value)).map((a) => a.name + ": " + a.value),
  ];
  console.log(`\n=== ${page}: ${leftover.length} untranslated`);
  for (const l of leftover) console.log("   " + l.replace(/\s+/g, " "));
  bad += leftover.length;
}

// every t() key used in the JS must exist in the en table
const en = vm.runInNewContext(src.slice(src.indexOf("const dict")) + "\n dict.en", {
  document: { documentElement: {}, body: {}, createTreeWalker: () => ({ nextNode: () => null }), querySelectorAll: () => [] },
  navigator: {}, localStorage: { getItem: () => "ko" }, location: {}, NodeFilter: { SHOW_TEXT: 4 },
});
const missing = new Set();
for (const f of ["web/static/app.js", "web/static/admin.js"]) {
  const js = fs.readFileSync(f, "utf8");
  for (const m of js.matchAll(/\bt\("((?:[^"\\]|\\.)*)"/g)) if (!(m[1] in en)) missing.add(f + " → " + m[1]);
}
// settingFields labels reach t() as a variable, so scan that table too
const adminSrc = fs.readFileSync("web/static/admin.js", "utf8");
const block = adminSrc.slice(adminSrc.indexOf("const settingFields"), adminSrc.indexOf("];", adminSrc.indexOf("const settingFields")));
for (const m of block.matchAll(/\[\s*"[^"]+",\s*"([^"]+)"/g)) {
  if (!(m[1] in en)) missing.add("admin.js settingFields \u2192 " + m[1]);
}

console.log(`\n=== t() keys missing from en: ${missing.size}`);
for (const m of missing) console.log("   " + m);
process.exit(bad + missing.size ? 1 : 0);
