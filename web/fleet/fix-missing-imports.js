// fix-missing-imports.js
// Usage: node fix-missing-imports.js build-errors.txt src

const fs = require("fs");
const path = require("path");

const logPath = process.argv[2] || "build-errors.txt";
const srcRoot = process.argv[3] || "src";

if (!fs.existsSync(logPath)) {
  console.error(`Can't find ${logPath}. Save your build output there first:`);
  console.error(`  npm run build > ${logPath}`);
  process.exit(1);
}

const log = fs.readFileSync(logPath, "utf8");

// Collect missing names per file
const missingByFile = {};

function addMissing(file, name) {
  const rel = file.replace(/\\/g, "/");
  if (!missingByFile[rel]) missingByFile[rel] = new Set();
  missingByFile[rel].add(name);
}

// Pattern 1: Cannot find name 'X'
const re1 = /^(.+?)\(\d+,\d+\): error TS2304: Cannot find name '([^']+)'\./gm;
let m;
while ((m = re1.exec(log))) addMissing(m[1], m[2]);

// Pattern 2: 'X' cannot be used as a JSX component (e.g. Lock colliding with a global DOM type)
const re2 = /^(.+?)\(\d+,\d+\): error TS2786: '([^']+)' cannot be used as a JSX component\./gm;
while ((m = re2.exec(log))) addMissing(m[1], m[2]);

// Figure out which names are real lucide-react icons
let lucideExports;
try {
  lucideExports = new Set(Object.keys(require("lucide-react")));
} catch (e) {
  console.error("Couldn't load lucide-react. Run this from your project root (where node_modules lives).");
  process.exit(1);
}

// Figure out which toast import your codebase actually uses
function detectToastImport() {
  const candidates = ["sonner", "react-hot-toast"];
  const files = walk(srcRoot);
  for (const f of files) {
    const content = fs.readFileSync(f, "utf8");
    for (const pkg of candidates) {
      if (content.includes(`from "${pkg}"`) || content.includes(`from '${pkg}'`)) {
        return pkg;
      }
    }
  }
  return "sonner"; // fallback default
}

function walk(dir) {
  let results = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) results = results.concat(walk(full));
    else if (/\.(tsx|ts)$/.test(entry.name)) results.push(full);
  }
  return results;
}

const toastPkg = detectToastImport();
console.log(`Using "${toastPkg}" as the toast import source.\n`);

let filesChanged = 0;
let iconsAdded = 0;
const unresolvedReport = [];

for (const [file, namesSet] of Object.entries(missingByFile)) {
  const names = [...namesSet];
  if (!fs.existsSync(file)) continue;

  let content = fs.readFileSync(file, "utf8");
  const iconNames = names.filter((n) => lucideExports.has(n));
  const needsToast = names.includes("toast");
  const unresolved = names.filter((n) => !lucideExports.has(n) && n !== "toast");

  let changed = false;

  // Merge/insert lucide-react import
  if (iconNames.length) {
    const lucideRe = /import\s*{([^}]*)}\s*from\s*["']lucide-react["'];?/;
    const existing = content.match(lucideRe);
    if (existing) {
      const current = existing[1]
        .split(",")
        .map((s) => s.trim())
        .filter(Boolean);
      const merged = [...new Set([...current, ...iconNames])].sort();
      content = content.replace(lucideRe, `import { ${merged.join(", ")} } from "lucide-react";`);
    } else {
      const sorted = [...iconNames].sort();
      const importLine = `import { ${sorted.join(", ")} } from "lucide-react";\n`;
      content = insertAfterLastImport(content, importLine);
    }
    iconsAdded += iconNames.length;
    changed = true;
  }

  // Add toast import if missing
  if (needsToast && !new RegExp(`import\\s*{[^}]*\\btoast\\b[^}]*}\\s*from\\s*["']${toastPkg}["']`).test(content)) {
    const importLine = `import { toast } from "${toastPkg}";\n`;
    content = insertAfterLastImport(content, importLine);
    changed = true;
  }

  if (changed) {
    fs.writeFileSync(file, content, "utf8");
    filesChanged++;
  }

  if (unresolved.length) {
    unresolvedReport.push({ file, unresolved });
  }
}

function insertAfterLastImport(content, importLine) {
  const importRe = /^import .+$/gm;
  let lastMatch = null;
  let mm;
  while ((mm = importRe.exec(content))) lastMatch = mm;
  if (!lastMatch) return importLine + content;
  const insertPos = lastMatch.index + lastMatch[0].length;
  return content.slice(0, insertPos) + "\n" + importLine.trimEnd() + content.slice(insertPos);
}

console.log(`Files changed: ${filesChanged}`);
console.log(`Icon imports added: ${iconsAdded}`);

if (unresolvedReport.length) {
  console.log(`\nNames that aren't lucide-react icons — needs a manual look:`);
  for (const { file, unresolved } of unresolvedReport) {
    console.log(`  ${file}: ${unresolved.join(", ")}`);
  }
}