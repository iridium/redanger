// Java class file parser (JVMS chapter 4). Only what the interpreter needs.

export const ACC = {
  PUBLIC: 0x1,
  PRIVATE: 0x2,
  PROTECTED: 0x4,
  STATIC: 0x8,
  FINAL: 0x10,
  SYNCHRONIZED: 0x20,
  VOLATILE: 0x40,
  NATIVE: 0x100,
  INTERFACE: 0x200,
  ABSTRACT: 0x400,
};

// Modified UTF-8 -> JS string.
function mutf8(b, p, len) {
  let s = "";
  const end = p + len;
  while (p < end) {
    const c = b[p++];
    if (c < 0x80) s += String.fromCharCode(c);
    else if ((c & 0xe0) === 0xc0) s += String.fromCharCode(((c & 0x1f) << 6) | (b[p++] & 0x3f));
    else {
      const c2 = b[p++],
        c3 = b[p++];
      s += String.fromCharCode(((c & 0x0f) << 12) | ((c2 & 0x3f) << 6) | (c3 & 0x3f));
    }
  }
  return s;
}

export function parseClass(bytes) {
  const dv = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  let p = 0;
  const u1 = () => bytes[p++];
  const u2 = () => {
    const v = dv.getUint16(p);
    p += 2;
    return v;
  };
  const u4 = () => {
    const v = dv.getUint32(p);
    p += 4;
    return v;
  };
  if (u4() !== 0xcafebabe) throw new Error("bad class magic");
  p += 4; // version

  const n = u2();
  const cp = new Array(n);
  for (let i = 1; i < n; i++) {
    const tag = u1();
    switch (tag) {
      case 1: {
        const len = u2();
        cp[i] = { tag, value: mutf8(bytes, p, len) };
        p += len;
        break;
      }
      case 3:
        cp[i] = { tag, value: dv.getInt32(p) };
        p += 4;
        break;
      case 4:
        cp[i] = { tag, value: dv.getFloat32(p) };
        p += 4;
        break;
      case 5:
        cp[i] = { tag, value: dv.getBigInt64(p) };
        p += 8;
        i++;
        break;
      case 6:
        cp[i] = { tag, value: dv.getFloat64(p) };
        p += 8;
        i++;
        break;
      case 7:
        cp[i] = { tag, nameIndex: u2(), name: null, cls: null };
        break;
      case 8:
        cp[i] = { tag, stringIndex: u2() };
        break;
      case 9:
      case 10:
      case 11:
        // the interpreter's resolution caches are declared up front so refs keep one shape
        cp[i] = {
          tag,
          classIndex: u2(),
          ntIndex: u2(),
          className: null,
          name: null,
          desc: null,
          field: null,
          key: null,
          method: null,
          nd: null,
          icCls: null,
          icM: null,
        };
        break;
      case 12:
        cp[i] = { tag, nameIndex: u2(), descIndex: u2() };
        break;
      case 15:
        cp[i] = { tag, kind: u1(), ref: u2() };
        break;
      case 16:
        cp[i] = { tag, descIndex: u2() };
        break;
      case 18:
        cp[i] = { tag, bsm: u2(), ntIndex: u2() };
        break;
      default:
        throw new Error(`bad constant tag ${tag} at ${i}`);
    }
  }
  const utf = (i) => cp[i].value;
  // Resolve symbolic references into plain strings once.
  for (let i = 1; i < n; i++) {
    const e = cp[i];
    if (!e) continue;
    if (e.tag === 7) e.name = utf(e.nameIndex);
    else if (e.tag === 8) e.string = utf(e.stringIndex);
    else if (e.tag === 12) {
      e.name = utf(e.nameIndex);
      e.desc = utf(e.descIndex);
    }
  }
  for (let i = 1; i < n; i++) {
    const e = cp[i];
    if (e && (e.tag === 9 || e.tag === 10 || e.tag === 11)) {
      e.className = cp[e.classIndex].name;
      e.name = cp[e.ntIndex].name;
      e.desc = cp[e.ntIndex].desc;
    }
  }

  const access = u2();
  const name = cp[u2()].name;
  const si = u2();
  const superName = si ? cp[si].name : null;
  const interfaces = [];
  for (let i = u2(); i > 0; i--) interfaces.push(cp[u2()].name);

  const readAttrs = (onAttr) => {
    for (let i = u2(); i > 0; i--) {
      const aname = utf(u2());
      const len = u4();
      const end = p + len;
      onAttr(aname, len);
      p = end;
    }
  };

  const fields = [];
  for (let i = u2(); i > 0; i--) {
    const f = { access: u2(), name: utf(u2()), desc: null, constant: undefined };
    f.desc = utf(u2());
    readAttrs((a) => {
      if (a === "ConstantValue") f.constant = u2();
    });
    fields.push(f);
  }

  const methods = [];
  for (let i = u2(); i > 0; i--) {
    const m = { access: u2(), name: utf(u2()), desc: null, code: null };
    m.desc = utf(u2());
    readAttrs((a) => {
      if (a !== "Code") return;
      const maxStack = u2(),
        maxLocals = u2();
      const clen = u4();
      const code = bytes.slice(p, p + clen);
      p += clen;
      const handlers = [];
      for (let k = u2(); k > 0; k--) {
        handlers.push({ start: u2(), end: u2(), handler: u2(), type: 0 });
        handlers[handlers.length - 1].type = u2();
      }
      const lines = [];
      readAttrs((ca) => {
        if (ca === "LineNumberTable") for (let k = u2(); k > 0; k--) lines.push([u2(), u2()]);
      });
      m.code = { maxStack, maxLocals, code, handlers, lines };
    });
    methods.push(m);
  }

  let sourceFile = null;
  readAttrs((a) => {
    if (a === "SourceFile") sourceFile = utf(u2());
  });

  return { access, name, superName, interfaces, cp, fields, methods, sourceFile };
}

// "(ILjava/lang/String;[J)V" -> {args: ["I","Ljava/lang/String;","[J"], ret: "V", slots}
export function parseDesc(desc) {
  const args = [];
  let i = 1,
    slots = 0;
  while (desc[i] !== ")") {
    const start = i;
    while (desc[i] === "[") i++;
    if (desc[i] === "L") i = desc.indexOf(";", i);
    i++;
    const t = desc.slice(start, i);
    args.push(t);
    slots += t === "J" || t === "D" ? 2 : 1;
  }
  return { args, ret: desc.slice(i + 1), slots };
}
