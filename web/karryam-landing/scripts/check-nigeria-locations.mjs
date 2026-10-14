import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const source = readFileSync(new URL("../src/lib/nigeria-locations.ts", import.meta.url), "utf8");
const launch = readFileSync(new URL("../src/components/Launch.tsx", import.meta.url), "utf8");
const faq = readFileSync(new URL("../src/components/Faq.tsx", import.meta.url), "utf8");

function stringArray(name) {
  const marker = `export const ${name} = [`;
  const start = source.indexOf(marker);
  assert.notEqual(start, -1, `Missing array: ${name}`);
  const end = source.indexOf("] as const;", start);
  assert.notEqual(end, -1, `Unterminated array: ${name}`);
  return [...source.slice(start + marker.length, end).matchAll(/"([^"]+)"/g)]
    .map((value) => value[1]);
}

const constitutionalStates = [
  "Abia",
  "Adamawa",
  "Akwa Ibom",
  "Anambra",
  "Bauchi",
  "Bayelsa",
  "Benue",
  "Borno",
  "Cross River",
  "Delta",
  "Ebonyi",
  "Edo",
  "Ekiti",
  "Enugu",
  "Gombe",
  "Imo",
  "Jigawa",
  "Kaduna",
  "Kano",
  "Katsina",
  "Kebbi",
  "Kogi",
  "Kwara",
  "Lagos",
  "Nasarawa",
  "Niger",
  "Ogun",
  "Ondo",
  "Osun",
  "Oyo",
  "Plateau",
  "Rivers",
  "Sokoto",
  "Taraba",
  "Yobe",
  "Zamfara",
];
const states = stringArray("NIGERIA_STATES");
assert.equal(states.length, 36);
assert.equal(new Set(states).size, 36);
assert.deepEqual(states, constitutionalStates);
assert.ok(source.includes('export const FCT = "Federal Capital Territory (Abuja)" as const;'));
assert.ok(source.includes("...NIGERIA_STATES.slice(0, 14)"));
assert.ok(source.includes("...NIGERIA_STATES.slice(14)"));
const all = [...states.slice(0, 14), "Federal Capital Territory (Abuja)", ...states.slice(14)];
assert.equal(all.length, 37);
assert.equal(new Set(all).size, 37);
const featured = stringArray("FEATURED_LOCATIONS");
assert.deepEqual(featured, ["Lagos", "Abuja", "Port Harcourt", "Oyo State"]);
assert.match(launch, /FEATURED_LOCATIONS\.map/);
assert.match(launch, /NIGERIA_STATES_AND_FCT\.map/);
assert.match(launch, /aria-expanded=\{showAllLocations\}/);
assert.match(launch, /hidden=\{!showAllLocations\}/);
assert.match(faq, /Oyo State/);
console.log("PASS: 36 unique states, separate FCT, 4 featured locations, expandable directory, matching FAQ");
