import fs from "node:fs";
import path from "node:path";
import vm from "node:vm";

const root = path.resolve(import.meta.dirname, "..");
const webRoot = path.join(root, "web");
const htmlFiles = fs.readdirSync(webRoot).filter((name) => name.endsWith(".html"));
const failures = [];

const fail = (file, message) => failures.push(`${file}: ${message}`);
const escapeRE = (value) => value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

for (const file of htmlFiles) {
  const fullPath = path.join(webRoot, file);
  const source = fs.readFileSync(fullPath, "utf8");

  for (const match of source.matchAll(/<script(?![^>]*\bsrc=)[^>]*>([\s\S]*?)<\/script>/gi)) {
    try {
      new vm.Script(`(function(){\n${match[1]}\n})`, { filename: file });
    } catch (error) {
      fail(file, `inline JavaScript does not parse: ${error.message}`);
    }
  }

  // Only IDs present in the saved document can collide at initial render.
  // Template strings inside scripts are checked through their behavior hooks
  // below and often describe mutually exclusive screens.
  const documentMarkup = source.replace(/<script\b[^>]*>[\s\S]*?<\/script>/gi, "");
  const seenIDs = new Set();
  for (const match of documentMarkup.matchAll(/\bid="([^"]+)"/g)) {
    if (seenIDs.has(match[1])) fail(file, `duplicate static id ${match[1]}`);
    seenIDs.add(match[1]);
  }

  for (const match of source.matchAll(/(?:src|href)="(\/[^"]+)"/g)) {
    const urlPath = match[1].split(/[?#]/)[0];
    if (urlPath.startsWith("/api/")) continue;
    const diskPath = path.join(webRoot, urlPath);
    if (!fs.existsSync(diskPath)) fail(file, `missing local asset ${urlPath}`);
  }

  if (/href="(?:#|javascript:)/i.test(source)) fail(file, "contains a placeholder or javascript: link");

  for (const match of source.matchAll(/<button([^>]*)\bid="([^"]+)"([^>]*)>/gi)) {
    const attrs = `${match[1]} ${match[3]}`;
    if (/type="submit"/i.test(attrs) || /value="cancel"/i.test(attrs)) continue;
    const occurrences = (source.match(new RegExp(escapeRE(match[2]), "g")) || []).length;
    if (occurrences < 2) fail(file, `button #${match[2]} has no visible script reference`);
  }
}

const app = fs.readFileSync(path.join(webRoot, "app.html"), "utf8");
for (const match of app.matchAll(/<button[^>]*\b(data-[a-z0-9-]+)=/gi)) {
  const attribute = match[1];
  const datasetName = attribute.slice(5).replace(/-([a-z])/g, (_, letter) => letter.toUpperCase());
  const selectorUses = (app.match(new RegExp(escapeRE(attribute), "g")) || []).length;
  const datasetUses = (app.match(new RegExp(`dataset\\.${escapeRE(datasetName)}`, "g")) || []).length;
  if (selectorUses + datasetUses < 2) fail("app.html", `${attribute} appears on a button but has no matching behavior hook`);
}

const navViews = new Set([...app.matchAll(/class="nav-item"[^>]*data-view="([^"]+)"/g)].map((match) => match[1]));
const routedViews = new Set([...app.matchAll(/name === "([^"]+)"/g)].map((match) => match[1]));
for (const view of navViews) {
  if (!routedViews.has(view)) fail("app.html", `navigation view ${view} has no render route`);
}

if (failures.length) {
  console.error(failures.join("\n"));
  process.exit(1);
}

console.log(`UI audit passed: ${htmlFiles.length} HTML surfaces, local links/assets, inline scripts, IDs, navigation routes, and button hooks.`);
