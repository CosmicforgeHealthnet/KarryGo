const fs = require("fs");
const path = require("path");

const args = process.argv.slice(2);
const targetDir = args.find((a) => !a.startsWith("--")) || "./src";
const dryRun = args.includes("--dry");

const IMPORT_RE =
  /^import\s+(type\s+)?\{([^}]*)\}\s+from\s+['"]([^'"]+)['"];?\s*$/gm;

function walk(dir, exts, out = []) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    if (entry.name === "node_modules" || entry.name.startsWith(".")) continue;
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) walk(full, exts, out);
    else if (exts.some((e) => entry.name.endsWith(e))) out.push(full);
  }
  return out;
}

function parseSpecifiers(raw) {
  return raw
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean)
    .map((s) => {
      const m = s.match(/^(\S+)\s+as\s+(\S+)$/);
      if (m) return { imported: m[1], local: m[2], raw: s };
      return { imported: s, local: s, raw: s };
    });
}

function fixFile(filePath) {
  const src = fs.readFileSync(filePath, "utf8");
  const matches = [...src.matchAll(IMPORT_RE)];
  if (matches.length === 0) return null;

  const byModule = new Map();
  const allMatchInfo = [];

  for (const m of matches) {
    const [, typeFlag, specsRaw, moduleName] = m;
    allMatchInfo.push({ match: m, module: moduleName });

    if (!byModule.has(moduleName)) {
      byModule.set(moduleName, {
        isType: !!typeFlag,
        specifiers: new Map(),
        firstIndex: m.index,
      });
    }
    const entry = byModule.get(moduleName);
    for (const spec of parseSpecifiers(specsRaw)) {
      if (!entry.specifiers.has(spec.local)) {
        entry.specifiers.set(spec.local, spec);
      }
    }
  }

  const modulesWithDupes = new Set();
  for (const { module } of allMatchInfo) {
    const count = allMatchInfo.filter((x) => x.module === module).length;
    if (count > 1) modulesWithDupes.add(module);
  }
  for (const m of matches) {
    const specs = parseSpecifiers(m[2]);
    const seen = new Set();
    for (const s of specs) {
      if (seen.has(s.local)) modulesWithDupes.add(m[3]);
      seen.add(s.local);
    }
  }

  if (modulesWithDupes.size === 0) return null;

  let out = src;

  for (let i = allMatchInfo.length - 1; i >= 0; i--) {
    const { match, module } = allMatchInfo[i];
    if (!modulesWithDupes.has(module)) continue;

    const entry = byModule.get(module);
    const isFirstOccurrence = match.index === entry.firstIndex;

    if (isFirstOccurrence) {
      const specList = [...entry.specifiers.values()]
        .map((s) => s.raw)
        .join(", ");
      const typePrefix = entry.isType ? "type " : "";
      const replacement = `import ${typePrefix}{ ${specList} } from '${module}';`;
      out =
        out.slice(0, match.index) +
        replacement +
        out.slice(match.index + match[0].length);
    } else {
      let end = match.index + match[0].length;
      if (out[end] === "\n") end += 1;
      out = out.slice(0, match.index) + out.slice(end);
    }
  }

  return out === src ? null : out;
}

function main() {
  const absDir = path.resolve(targetDir);
  if (!fs.existsSync(absDir)) {
    console.error(`Directory not found: ${absDir}`);
    process.exit(1);
  }
  const files = walk(absDir, [".tsx", ".ts"]);
  let changed = 0;

  for (const file of files) {
    const fixed = fixFile(file);
    if (fixed !== null) {
      changed++;
      console.log(`${dryRun ? "[dry] " : ""}fixed: ${file}`);
      if (!dryRun) fs.writeFileSync(file, fixed, "utf8");
    }
  }

  console.log(`\nDone. ${changed} file(s) ${dryRun ? "would be " : ""}modified out of ${files.length} scanned.`);
  if (dryRun) console.log("Run again without --dry to write changes.");
}

main();
