// Large lazily read files (the SD card, rt.jar) are published as fixed-size chunk files,
// <file>.0, <file>.1, ..., fetched whole: no HTTP range requests, and no file over the size
// limits of static hosts like Cloudflare Pages (25 MiB). Files up to one chunk stay whole.
export const CHUNK = 1 << 20;

// Paths under the site root that are read lazily, and so get chunked.
export const isLazy = (path) => /^vendor\/(card\/|rt\.jar$)/.test(path);
