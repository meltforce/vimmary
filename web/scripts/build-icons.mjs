/**
 * Builds the app icon set in public/app-icon/ from the mark defined below.
 *
 * Run it with `npm run icons`. It is not part of `npm run build`: the output is
 * committed, the icons change once a year at most, and the script needs Chrome
 * and ImageMagick, neither of which CI has.
 *
 *   node scripts/build-icons.mjs
 *   CHROME=/path/to/chrome node scripts/build-icons.mjs
 *
 * The mark is "vm" in Archivo 800, the page ground (#f3f2f2) on a full-bleed
 * accent field (#ec3013), with a rule bar beneath it on the same measure.
 *
 * The mark must stay LIGHTER than the field. iOS 18 derives the dark and tinted
 * home screen variants from this one file; with a dark mark on a mid field both
 * collapse toward black and the icon reads as an empty rounded square. This is
 * the defect FreeReps fixed in c7319b0 and the reason the artwork is not ink on
 * accent.
 *
 * No rounded corners are baked in — iOS and Android apply their own mask.
 */

import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const web = join(here, "..");
const outDir = join(web, "public", "app-icon");
const tmpDir = join(here, ".icon-build");

const CHROME =
  process.env.CHROME ??
  "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";

const FIELD = "#ec3013";
const MARK = "#f3f2f2";

/* Geometry on the 512 canvas, measured rather than guessed: at font-size 280
   the x-height block of "vm" is 397 × 151, which is the widest the mark can be
   and still leave a margin on both edges. The bar takes the mark's measure. */
const GEOM = {
  fontSize: 280,
  baseline: 302,
  barX: 54,
  barY: 336,
  barW: 397,
  barH: 30,
};

function svg({ scale = 1 } = {}) {
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512" width="512" height="512">
  <rect width="512" height="512" fill="${FIELD}"/>
  <g transform="translate(256 256) scale(${scale}) translate(-256 -256)">
    <text x="256" y="${GEOM.baseline}" text-anchor="middle" fill="${MARK}"
          font-family="Archivo Variable" font-weight="800"
          font-size="${GEOM.fontSize}" letter-spacing="-0.03em">vm</text>
    <rect x="${GEOM.barX}" y="${GEOM.barY}" width="${GEOM.barW}" height="${GEOM.barH}" fill="${MARK}"/>
  </g>
</svg>`;
}

/* Chrome renders the SVG, so the mark comes out in the real variable face
   rather than a fallback. The font is inlined because the render runs from a
   file:// URL. */
const font = readFileSync(
  join(
    web,
    "node_modules/@fontsource-variable/archivo/files/archivo-latin-wght-normal.woff2",
  ),
).toString("base64");

function page(markup) {
  return `<!DOCTYPE html><html><head><meta charset="utf-8"><style>
@font-face{font-family:"Archivo Variable";font-style:normal;font-weight:100 900;
  src:url(data:font/woff2;base64,${font}) format("woff2-variations");}
html,body{margin:0;padding:0;width:512px;height:512px;overflow:hidden}
svg{display:block}
</style></head><body>${markup}</body></html>`;
}

function render(name, markup) {
  const html = join(tmpDir, `${name}.html`);
  const png = join(tmpDir, `${name}.png`);
  writeFileSync(html, page(markup));
  execFileSync(CHROME, [
    "--headless",
    "--disable-gpu",
    `--screenshot=${png}`,
    "--window-size=512,512",
    "--hide-scrollbars",
    "--force-device-scale-factor=1",
    `file://${html}`,
  ], { stdio: "ignore" });
  return png;
}

function resize(from, to, size) {
  execFileSync("magick", [from, "-resize", `${size}x${size}`, to]);
}

rmSync(tmpDir, { recursive: true, force: true });
mkdirSync(tmpDir, { recursive: true });
mkdirSync(outDir, { recursive: true });

const full = render("icon", svg());
/* Android maskable icons are cropped to a shape that can eat the outer 20%, so
   the mark shrinks into the safe zone while the field keeps bleeding. */
const maskable = render("maskable", svg({ scale: 0.78 }));

execFileSync("magick", [full, join(outDir, "icon-512.png")]);
execFileSync("magick", [maskable, join(outDir, "icon-maskable-512.png")]);
resize(full, join(outDir, "icon-192.png"), 192);
resize(full, join(outDir, "apple-touch-icon-180.png"), 180);
resize(full, join(outDir, "favicon-32.png"), 32);

rmSync(tmpDir, { recursive: true, force: true });

console.log(`wrote 5 icons to ${outDir}`);
