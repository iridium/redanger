// Minimal jar/zip reader. Entries are inflated lazily through a synchronous `inflateRaw`
// (node:zlib in Node; in the browser, jars are pre-inflated with DecompressionStream).

export class Jar {
  constructor(bytes, inflateRaw) {
    this.bytes = bytes;
    this.inflateRaw = inflateRaw;
    this.entries = new Map(); // name -> {method, offset, csize, size} or Uint8Array once inflated
    this.readDirectory();
  }

  readDirectory() {
    const b = this.bytes;
    const dv = new DataView(b.buffer, b.byteOffset, b.byteLength);
    let eocd = b.length - 22;
    while (eocd >= 0 && dv.getUint32(eocd, true) !== 0x06054b50) eocd--;
    if (eocd < 0) throw new Error("not a zip file");
    const count = dv.getUint16(eocd + 10, true);
    let p = dv.getUint32(eocd + 16, true);
    const dec = new TextDecoder();
    for (let i = 0; i < count; i++) {
      const method = dv.getUint16(p + 10, true);
      const csize = dv.getUint32(p + 20, true);
      const size = dv.getUint32(p + 24, true);
      const nlen = dv.getUint16(p + 28, true);
      const elen = dv.getUint16(p + 30, true);
      const clen = dv.getUint16(p + 32, true);
      const local = dv.getUint32(p + 42, true);
      const name = dec.decode(b.subarray(p + 46, p + 46 + nlen));
      this.entries.set(name, { method, local, csize, size });
      p += 46 + nlen + elen + clen;
    }
  }

  has(name) {
    return this.entries.has(name);
  }

  raw(e) {
    const dv = new DataView(this.bytes.buffer, this.bytes.byteOffset, this.bytes.byteLength);
    const start =
      e.local + 30 + dv.getUint16(e.local + 26, true) + dv.getUint16(e.local + 28, true);
    return this.bytes.subarray(start, start + e.csize);
  }

  get(name) {
    const e = this.entries.get(name);
    if (!e) return null;
    if (e instanceof Uint8Array) return e;
    const data = e.method === 0 ? this.raw(e) : this.inflateRaw(this.raw(e));
    this.entries.set(name, data);
    return data;
  }

  // Browser: inflate every entry up front so later reads are synchronous.
  async inflateAll(filter = () => true) {
    for (const [name, e] of this.entries) {
      if (e instanceof Uint8Array || !filter(name)) continue;
      if (e.method === 0) {
        this.entries.set(name, this.raw(e));
        continue;
      }
      const ds = new DecompressionStream("deflate-raw");
      const out = new Response(new Blob([this.raw(e)]).stream().pipeThrough(ds));
      this.entries.set(name, new Uint8Array(await out.arrayBuffer()));
    }
  }
}
