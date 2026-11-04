// Raw DEFLATE decoder (RFC 1951), synchronous, for java.util.zip.Inflater.
// inflate(bytes) -> {out: Uint8Array, used: bytes consumed} or throws Incomplete when the
// input ends early.

export class Incomplete extends Error {}

const LBASE = [
  3, 4, 5, 6, 7, 8, 9, 10, 11, 13, 15, 17, 19, 23, 27, 31, 35, 43, 51, 59, 67, 83, 99, 115, 131,
  163, 195, 227, 258,
];
const LEXT = [
  0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3, 3, 4, 4, 4, 4, 5, 5, 5, 5, 0,
];
const DBASE = [
  1, 2, 3, 4, 5, 7, 9, 13, 17, 25, 33, 49, 65, 97, 129, 193, 257, 385, 513, 769, 1025, 1537, 2049,
  3073, 4097, 6145, 8193, 12289, 16385, 24577,
];
const DEXT = [
  0, 0, 0, 0, 1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 6, 7, 7, 8, 8, 9, 9, 10, 10, 11, 11, 12, 12, 13, 13,
];
const CLORDER = [16, 17, 18, 0, 8, 7, 9, 6, 10, 5, 11, 4, 12, 3, 13, 2, 14, 1, 15];

// Canonical Huffman table: counts per length + symbols sorted by code.
function build(lengths) {
  const counts = new Uint16Array(16);
  for (const l of lengths) counts[l]++;
  counts[0] = 0;
  const offs = new Uint16Array(16);
  for (let i = 1; i < 16; i++) offs[i] = offs[i - 1] + counts[i - 1];
  const symbols = new Uint16Array(lengths.length);
  lengths.forEach((l, s) => {
    if (l) symbols[offs[l]++] = s;
  });
  return { counts, symbols };
}

let FIXED_L, FIXED_D;
function fixed() {
  if (!FIXED_L) {
    const l = new Uint8Array(288);
    l.fill(8, 0, 144);
    l.fill(9, 144, 256);
    l.fill(7, 256, 280);
    l.fill(8, 280, 288);
    FIXED_L = build(l);
    FIXED_D = build(new Uint8Array(30).fill(5));
  }
  return [FIXED_L, FIXED_D];
}

export function inflate(src, sizeHint = 0) {
  let pos = 0,
    bit = 0,
    nbits = 0;
  let out = new Uint8Array(Math.max(1024, sizeHint));
  let n = 0;
  const need = (k) => {
    while (nbits < k) {
      if (pos >= src.length) throw new Incomplete();
      bit |= src[pos++] << nbits;
      nbits += 8;
    }
  };
  const bits = (k) => {
    if (!k) return 0;
    need(k);
    const v = bit & ((1 << k) - 1);
    bit >>>= k;
    nbits -= k;
    return v;
  };
  const decode = (h) => {
    let code = 0,
      first = 0,
      index = 0;
    for (let len = 1; len < 16; len++) {
      code |= bits(1);
      const count = h.counts[len];
      if (code - count < first) return h.symbols[index + (code - first)];
      index += count;
      first = (first + count) << 1;
      code <<= 1;
    }
    throw new Error("bad huffman code");
  };
  const put = (b) => {
    if (n === out.length) {
      const o = new Uint8Array(out.length * 2);
      o.set(out);
      out = o;
    }
    out[n++] = b;
  };

  let last = 0;
  while (!last) {
    last = bits(1);
    const type = bits(2);
    if (type === 0) {
      bit = 0;
      nbits = 0; // byte align
      if (pos + 4 > src.length) throw new Incomplete();
      const len = src[pos] | (src[pos + 1] << 8);
      pos += 4;
      if (pos + len > src.length) throw new Incomplete();
      for (let i = 0; i < len; i++) put(src[pos++]);
      continue;
    }
    let lt, dt;
    if (type === 1) [lt, dt] = fixed();
    else if (type === 2) {
      const hlit = bits(5) + 257,
        hdist = bits(5) + 1,
        hclen = bits(4) + 4;
      const cl = new Uint8Array(19);
      for (let i = 0; i < hclen; i++) cl[CLORDER[i]] = bits(3);
      const ct = build(cl);
      const lens = new Uint8Array(hlit + hdist);
      for (let i = 0; i < hlit + hdist;) {
        const sym = decode(ct);
        if (sym < 16) lens[i++] = sym;
        else if (sym === 16) {
          const p = lens[i - 1];
          for (let r = 3 + bits(2); r > 0; r--) lens[i++] = p;
        } else if (sym === 17) i += 3 + bits(3);
        else i += 11 + bits(7);
      }
      lt = build(lens.subarray(0, hlit));
      dt = build(lens.subarray(hlit));
    } else throw new Error("bad block type");
    for (;;) {
      const sym = decode(lt);
      if (sym < 256) put(sym);
      else if (sym === 256) break;
      else {
        const s = sym - 257;
        const len = LBASE[s] + bits(LEXT[s]);
        const ds = decode(dt);
        const dist = DBASE[ds] + bits(DEXT[ds]);
        for (let i = 0; i < len; i++) put(out[n - dist]);
      }
    }
  }
  return { out: out.subarray(0, n), used: pos - (nbits >> 3) };
}
