// Outlets send their photos at full size: half a megabyte for a picture shown 72 pixels wide. So no
// page loads one directly. Each goes through a resizing service that returns it at the width it is
// shown at, as WebP, cached at the edge: a thumbnail drops from hundreds of kilobytes to about five.
// The service is wsrv.nl (images.weserv.nl): free, open source, no key. It is sent the photo's
// public address and nothing else. If it is down the photo fails to load and StoryArt shows the
// story's generated art instead. To move to another resizer, change RESIZER and the query below.

const RESIZER = 'https://wsrv.nl/';
const QUALITY = '70';
const FORMAT = 'webp';
const DENSITY = 2; // photos are asked for at twice their shown width, for sharp phone and laptop screens

/** Shown widths in CSS pixels. A thumbnail sits beside a headline; a banner fills a column or a phone's width. */
export const PICTURE_THUMB = 96;
export const PICTURE_BANNER = 400;

/**
 * The address of a photo resized to the width it is shown at.
 * @param {string} url the outlet's own photo
 * @param {number} width the widest it is shown, in CSS pixels
 */
export function sized(url, width) {
  // "we" is "without enlargement": a photo already smaller than asked is left as it is.
  return `${RESIZER}?${new URLSearchParams({ url, w: String(width * DENSITY), output: FORMAT, q: QUALITY, we: '' })}`;
}
