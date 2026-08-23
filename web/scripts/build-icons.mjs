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
 * The mark is "vm" in Bricolage Grotesque 700, amber (#d9a066) on a full-bleed
 * ink field (#241f1a), with a rule bar beneath it on the same measure. The
 * Shelf redesign re-derived it from the new palette; the #ec3013 field was the
 * shared Modernist accent, which vimmary no longer uses.
 *
 * The mark must stay LIGHTER than the field. iOS 18 derives the dark and tinted
 * home screen variants from this one file; with a dark mark on a mid field both
 * collapse toward black and the icon reads as an empty rounded square. This is
 * the defect FreeReps fixed in c7319b0, and the reason the pairing is amber on
 * ink rather than ink on amber.
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

const FIELD = "#241f1a";
const MARK = "#d9a066";

/* Geometry on the 512 canvas. The font size is the source ratio — 280 of 512 —
   and stays fixed across a change of face; the bar takes whatever measure the
   mark actually has, which is why `measureMark` reads it from the rendered
   face instead of a constant. Trimming the bar to a number measured for
   Archivo would have left it short of the new mark by ~20px. */
const GEOM = {
  fontSize: 280,
  baseline: 302,
  barY: 336,
  barH: 30,
};

function svg({ scale = 1, barX, barW } = {}) {
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512" width="512" height="512">
  <rect width="512" height="512" fill="${FIELD}"/>
  <g transform="translate(256 256) scale(${scale}) translate(-256 -256)">
    <text x="256" y="${GEOM.baseline}" text-anchor="middle" fill="${MARK}"
          font-family="Bricolage Grotesque Variable" font-weight="700"
          font-size="${GEOM.fontSize}" letter-spacing="-0.03em">vm</text>
    <rect x="${barX}" y="${GEOM.barY}" width="${barW}" height="${GEOM.barH}" fill="${MARK}"/>
  </g>
</svg>`;
}

/* Chrome renders the SVG, so the mark comes out in the real variable face
   rather than a fallback. The font is inlined because the render runs from a
   file:// URL. */
const font = readFileSync(
  join(
    web,
    "node_modules/@fontsource-variable/bricolage-grotesque/files/bricolage-grotesque-latin-wght-normal.woff2",
  ),
).toString("base64");

function page(markup) {
  return `<!DOCTYPE html><html><head><meta charset="utf-8"><style>
@font-face{font-family:"Bricolage Grotesque Variable";font-style:normal;font-weight:200 800;
  src:url(data:font/woff2;base64,${font}) format("woff2-variations");}
html,body{margin:0;padding:0;width:512px;height:512px;overflow:hidden}
svg{display:block}
</style></head><body>${markup}</body></html>`;
}

/* Chrome measures the mark, so the bar matches the face that actually renders.
   The page prints the advance width of "vm" at GEOM.fontSize; the bar is that
   width, centred on the canvas. */
function measureMark() {
  const html = join(tmpDir, "measure.html");
  writeFileSync(
    html,
    page(`<div id="m" style="position:absolute;visibility:hidden;white-space:pre;
      font-family:'Bricolage Grotesque Variable';font-weight:700;
      font-size:${GEOM.fontSize}px;letter-spacing:-0.03em">vm</div>
      <script>document.title = String(document.getElementById("m").getBoundingClientRect().width);</script>`),
  );
  const out = execFileSync(CHROME, [
    "--headless",
    "--disable-gpu",
    "--dump-dom",
    "--virtual-time-budget=2000",
    `file://${html}`,
  ]).toString();
  const m = out.match(/<title>([\d.]+)<\/title>/);
  if (!m) throw new Error("could not measure the mark — Chrome returned no width");
  const width = Math.round(Number(m[1]));
  return { barW: width, barX: Math.round((512 - width) / 2) };
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

const bar = measureMark();
console.log(`mark measures ${bar.barW}px at font-size ${GEOM.fontSize}`);

const full = render("icon", svg(bar));
/* Android maskable icons are cropped to a shape that can eat the outer 20%, so
   the mark shrinks into the safe zone while the field keeps bleeding. */
const maskable = render("maskable", svg({ ...bar, scale: 0.78 }));

execFileSync("magick", [full, join(outDir, "icon-512.png")]);
execFileSync("magick", [maskable, join(outDir, "icon-maskable-512.png")]);
resize(full, join(outDir, "icon-192.png"), 192);
resize(full, join(outDir, "apple-touch-icon-180.png"), 180);
resize(full, join(outDir, "favicon-32.png"), 32);

rmSync(tmpDir, { recursive: true, force: true });

console.log(`wrote 5 icons to ${outDir}`);
