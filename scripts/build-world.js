// Builds the world map the Stories map draws and the country list newsd reads, from one public
// domain file: Natural Earth's 1:110m countries (naturalearthdata.com, no rights reserved).
//
//   src/feed/world.json            every country as an SVG path on a Miller map, with a label point
//   engine/news/countries.json     every country's id, name and the words that mean it in a headline
//
// Run it again only when the shapes or the words change:
//   download geojson/ne_110m_admin_0_countries.geojson from github.com/nvkelso/natural-earth-vector
//   node scripts/build-world.js path/to/ne_110m_admin_0_countries.geojson
import { readFileSync, writeFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const MAP_FILE = join(ROOT, 'src', 'feed', 'world.json');
const COUNTRIES_FILE = join(ROOT, 'engine', 'news', 'countries.json');
// A Miller map: x follows longitude; y stretches toward the poles less than Mercator does, so the
// far north stays a sane size and the map is tall enough to fill a screen. The poles are cut off.
const MAP_WIDTH = 1000;
const LON_SPAN = 360;
const LON_MIN = -180;
const LAT_MAX = 84;
const LAT_MIN = -58;
const SCALE = MAP_WIDTH / LON_SPAN;
const RADIAN = Math.PI / 180;
const MILLER_STRETCH = 1.25;
const MILLER_SQUEEZE = 0.4;
const UNITS_PER_RADIAN = MAP_WIDTH / (2 * Math.PI);
const miller = (/** @type {number} */ lat) => MILLER_STRETCH * Math.log(Math.tan(Math.PI / 4 + MILLER_SQUEEZE * lat * RADIAN));
const MAP_HEIGHT = Math.round((miller(LAT_MAX) - miller(LAT_MIN)) * UNITS_PER_RADIAN);
const DECIMALS = 1;
const MIN_RING_POINTS = 4;
const NO_CODE = '-99';
const SKIP = new Set(['AQ']); // Antarctica is below the cut
const ABBREVIATED = /\./;
const NAME_OVERRIDES = { US: 'United States', CD: 'DR Congo', CG: 'Congo', LA: 'Laos', KP: 'North Korea', KR: 'South Korea', RU: 'Russia' };
/** Names that are also a US state or a common first name: only their other words count. */
const AMBIGUOUS_NAMES = new Set(['georgia']);
/** Places too small for the 1:110m shapes that still make financial news: drawn as a dot. */
const DOTS = [
  { id: 'SG', name: 'Singapore', lon: 103.82, lat: 1.35 },
  { id: 'HK', name: 'Hong Kong', lon: 114.17, lat: 22.32 },
];
/** Words besides the name that put a headline in a country: its people, its capital, its institutions. */
const ALIASES = {
  US: ['u.s.', 'america', 'american', 'americans', 'washington', 'white house', 'wall street', 'pentagon'],
  GB: ['britain', 'british', 'uk', 'u.k.', 'london', 'england', 'scotland', 'wales'],
  CN: ['chinese', 'beijing', 'shanghai'],
  RU: ['russian', 'moscow', 'kremlin', 'putin'],
  UA: ['ukrainian', 'kyiv', 'kiev', 'zelensky', 'zelenskyy'],
  DE: ['german', 'berlin', 'bundesbank'],
  FR: ['french', 'paris', 'macron'],
  JP: ['japanese', 'tokyo'],
  IN: ['indian', 'new delhi', 'mumbai', 'modi'],
  IL: ['israeli', 'tel aviv', 'jerusalem', 'netanyahu'],
  PS: ['gaza', 'palestinian', 'palestinians', 'west bank', 'hamas'],
  IR: ['iranian', 'tehran'],
  SA: ['saudi', 'riyadh'],
  KR: ['south korean', 'seoul'],
  KP: ['north korean', 'pyongyang'],
  TW: ['taiwanese', 'taipei'],
  CA: ['canadian', 'ottawa', 'toronto'],
  MX: ['mexican', 'mexico city'],
  BR: ['brazilian', 'brasilia'],
  AR: ['argentine', 'buenos aires', 'milei'],
  VE: ['venezuelan', 'caracas', 'maduro'],
  TR: ['turkish', 'ankara', 'istanbul', 'erdogan', 'turkiye'],
  EG: ['egyptian', 'cairo'],
  ZA: ['south african', 'johannesburg'],
  NG: ['nigerian', 'lagos'],
  AU: ['australian', 'sydney', 'canberra'],
  CH: ['swiss', 'zurich'],
  IT: ['italian', 'rome', 'milan'],
  ES: ['spanish', 'madrid'],
  NL: ['dutch', 'amsterdam'],
  SE: ['swedish', 'stockholm', 'riksbank'],
  NO: ['norwegian', 'oslo'],
  PL: ['polish', 'warsaw'],
  GR: ['greek', 'athens'],
  ID: ['indonesian', 'jakarta'],
  PK: ['pakistani', 'islamabad'],
  AE: ['uae', 'dubai', 'abu dhabi', 'emirati'],
  QA: ['qatari', 'doha'],
  IQ: ['iraqi', 'baghdad'],
  SY: ['syrian', 'damascus'],
  LB: ['lebanese', 'beirut', 'hezbollah'],
  YE: ['yemeni', 'houthi', 'houthis'],
  AF: ['afghan', 'kabul', 'taliban'],
  VN: ['vietnamese', 'hanoi'],
  TH: ['thai', 'bangkok'],
  PH: ['philippine', 'filipino', 'manila'],
  MY: ['malaysian', 'kuala lumpur'],
  SG: ['singaporean'],
  CU: ['cuban', 'havana'],
  CO: ['colombian', 'bogota'],
  CL: ['chilean'],
  PE: ['peruvian'],
  CD: ['drc', 'kinshasa', 'congolese'],
  GE: ['georgian', 'tbilisi'],
  CZ: ['czech', 'prague', 'czechia'],
  CI: ['ivory coast', 'cote divoire'],
  MM: ['burma'],
  TL: ['east timor'],
};

const source = process.argv[2];
if (!source) {
  console.error('Usage: node scripts/build-world.js path/to/ne_110m_admin_0_countries.geojson');
  process.exit(1);
}

const round = (/** @type {number} */ n) => Number(n.toFixed(DECIMALS));
const px = (/** @type {number} */ lon) => (lon - LON_MIN) * SCALE;
const py = (/** @type {number} */ lat) => (miller(LAT_MAX) - miller(Math.max(LAT_MIN, Math.min(LAT_MAX, lat)))) * UNITS_PER_RADIAN;

/** One ring as a relative SVG path: a third of the bytes of absolute coordinates. */
function ringPath(/** @type {number[][]} */ ring) {
  let [x, y] = [round(px(ring[0][0])), round(py(ring[0][1]))];
  let d = `M${x} ${y}`;
  for (const [lon, lat] of ring.slice(1)) {
    const [nx, ny] = [round(px(lon)), round(py(lat))];
    if (nx === x && ny === y) continue;
    d += `l${round(nx - x)} ${round(ny - y)}`;
    [x, y] = [nx, ny];
  }
  return `${d}z`;
}

/** @param {any} feature */
function shape(feature) {
  const polygons = feature.geometry.type === 'Polygon' ? [feature.geometry.coordinates] : feature.geometry.coordinates;
  return polygons.flat().filter((/** @type {number[][]} */ ring) => ring.length >= MIN_RING_POINTS).map(ringPath).join('');
}

/** Lowercase words and phrases that mean this country, the name first. */
function terms(/** @type {string} */ id, /** @type {string} */ name, /** @type {any} */ p) {
  const names = [name, p.NAME, p.NAME_LONG, p.ADMIN, p.NAME_EN].filter((n) => n && !ABBREVIATED.test(n));
  const all = [...names, ...(ALIASES[id] ?? [])].map((t) => t.toLowerCase()).filter((t) => !AMBIGUOUS_NAMES.has(t));
  return [...new Set(all)];
}

const world = JSON.parse(readFileSync(source, 'utf8'));
const map = [];
const countries = [];
for (const feature of world.features) {
  const p = feature.properties;
  const id = p.ISO_A2_EH;
  if (id === NO_CODE || SKIP.has(id)) continue;
  const name = NAME_OVERRIDES[id] ?? (ABBREVIATED.test(p.NAME) ? p.NAME_LONG : p.NAME);
  map.push({ id, name, d: shape(feature), x: round(px(p.LABEL_X)), y: round(py(p.LABEL_Y)) });
  countries.push({ id, name, terms: terms(id, name, p) });
}
for (const dot of DOTS) {
  map.push({ id: dot.id, name: dot.name, d: '', x: round(px(dot.lon)), y: round(py(dot.lat)) });
  countries.push({ id: dot.id, name: dot.name, terms: [...new Set([dot.name.toLowerCase(), ...(ALIASES[dot.id] ?? [])])] });
}
map.sort((a, b) => a.id.localeCompare(b.id));
countries.sort((a, b) => a.id.localeCompare(b.id));

writeFileSync(MAP_FILE, JSON.stringify({ width: MAP_WIDTH, height: MAP_HEIGHT, countries: map }));
writeFileSync(COUNTRIES_FILE, `${JSON.stringify({ countries }, null, 1)}\n`);
console.log(`${map.length} countries: ${MAP_FILE} (${readFileSync(MAP_FILE).length} bytes), ${COUNTRIES_FILE}`);
