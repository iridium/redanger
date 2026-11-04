// A small JVM interpreter. Threads are green threads with explicit frame stacks so the
// scheduler can switch between them; natives are JS functions keyed "pkg/Cls.name(desc)".
// Any Java method (not just ACC_NATIVE ones) can be overridden by registering a native.

import { parseClass, parseDesc, ACC } from "./classfile.js";

export class JavaThrow extends Error {
  constructor(obj) {
    super("JavaThrow " + (obj && obj.cls ? obj.cls.name : obj));
    this.obj = obj;
  }
}

// Returned by a native to say "this thread is now blocked; t.resume() produces the result".
export const BLOCK = Symbol("BLOCK");
// Returned by a native that pushed a Java frame whose return value becomes the native's result.
export const CALLED = Symbol("CALLED");
// Placeholder occupying the second stack/local slot of a long or double.
const TOP = null;

export class JObject {
  constructor(cls) {
    this.cls = cls;
    Object.assign(this, cls.template);
  }
}

export class JArray {
  constructor(cls, a) {
    this.cls = cls;
    this.a = a;
  }
}

const PRIM = {
  Z: "boolean",
  B: "byte",
  C: "char",
  S: "short",
  I: "int",
  J: "long",
  F: "float",
  D: "double",
  V: "void",
};

function defaultValue(desc) {
  const c = desc[0];
  return c === "J" ? 0n : c === "L" || c === "[" ? null : 0;
}

function newTyped(elem, n) {
  switch (elem) {
    case "Z":
    case "B":
      return new Int8Array(n);
    case "C":
      return new Uint16Array(n);
    case "S":
      return new Int16Array(n);
    case "I":
      return new Int32Array(n);
    case "J":
      return new BigInt64Array(n);
    case "F":
      return new Float32Array(n);
    case "D":
      return new Float64Array(n);
    default:
      return new Array(n).fill(null);
  }
}

export class JClass {
  constructor(name) {
    this.name = name;
    this.superClass = null;
    this.interfaces = [];
    this.fields = new Map(); // name -> field
    this.methods = new Map(); // name+desc -> method
    this.vcache = new Map();
    this.statics = {};
    this.template = {};
    this.state = "linked";
    this.access = ACC.PUBLIC;
    this.classObj = null;
    this.isArray = false;
    this.isPrimitive = false;
    this.component = null; // JClass of array elements
    this.cf = null;
  }
}

export class Thread {
  constructor(jvm, jthread, name) {
    this.jvm = jvm;
    this.jthread = jthread; // java.lang.Thread object
    this.name = name;
    this.frames = [];
    this.state = "runnable"; // runnable | blocked | waiting | sleeping | terminated
    this.wakeAt = Infinity;
    this.resume = null; // () => value | BLOCK, called when scheduled after BLOCK
    this.interrupted = false;
    this.id = jvm.nextThreadId++;
  }
}

export class JVM {
  constructor({ jars, natives = {}, log = console.log }) {
    this.jars = jars; // [Jar], searched in order
    this.natives = natives;
    this.log = log;
    this.classes = new Map();
    this.interned = new Map();
    this.threads = [];
    this.nextThreadId = 1;
    this.nextHash = 1;
    this.current = null;
    this.exited = false;
    this.missingNatives = new Set();
    this.fastForward = false; // see runSlice
  }

  findBytes(path) {
    for (const j of this.jars) {
      const b = j.get(path);
      if (b) return b;
    }
    return null;
  }

  loadClass(name) {
    let c = this.classes.get(name);
    if (c) return c;
    if (name[0] === "[") return this.arrayClass(name);
    const bytes = this.findBytes(name + ".class");
    if (!bytes) return null;
    const cf = parseClass(bytes);
    c = new JClass(name);
    c.cf = cf;
    c.access = cf.access;
    this.classes.set(name, c);
    if (cf.superName) {
      c.superClass = this.loadClass(cf.superName);
      if (!c.superClass) throw new Error(`superclass ${cf.superName} of ${name} not found`);
    }
    c.interfaces = cf.interfaces.map((i) => {
      const ic = this.loadClass(i);
      if (!ic) throw new Error(`interface ${i} of ${name} not found`);
      return ic;
    });
    this.link(c);
    c.state = "loaded";
    return c;
  }

  link(c) {
    const cf = c.cf;
    const inherited = c.superClass ? c.superClass.template : {};
    Object.assign(c.template, inherited);
    for (const f of cf.fields) {
      const field = { cls: c, name: f.name, desc: f.desc, access: f.access, key: f.name };
      if (f.access & ACC.STATIC) {
        c.statics[f.name] = defaultValue(f.desc);
        if (f.constant) {
          const k = cf.cp[f.constant];
          c.statics[f.name] = k.tag === 8 ? this.str(k.string) : k.value;
        }
      } else {
        if (field.key in inherited) field.key = f.name + "$" + c.name;
        c.template[field.key] = defaultValue(f.desc);
      }
      c.fields.set(f.name, field);
    }
    for (const m of cf.methods) {
      const d = parseDesc(m.desc);
      const method = {
        cls: c,
        name: m.name,
        desc: m.desc,
        access: m.access,
        code: m.code,
        args: d.args,
        ret: d.ret,
        argSlots: d.slots + (m.access & ACC.STATIC ? 0 : 1),
        native: undefined,
      };
      c.methods.set(m.name + m.desc, method);
    }
  }

  arrayClass(name) {
    let c = this.classes.get(name);
    if (c) return c;
    c = new JClass(name);
    c.isArray = true;
    c.superClass = this.loadClass("java/lang/Object");
    c.interfaces = [this.loadClass("java/lang/Cloneable"), this.loadClass("java/io/Serializable")];
    c.access = ACC.PUBLIC | ACC.FINAL | ACC.ABSTRACT;
    const e = name.slice(1);
    c.elem = e;
    c.component =
      e[0] === "L"
        ? this.loadClass(e.slice(1, -1))
        : e[0] === "["
          ? this.arrayClass(e)
          : this.primClass(e);
    c.state = "initialized";
    this.classes.set(name, c);
    return c;
  }

  primClass(d) {
    const key = "#" + d;
    let c = this.classes.get(key);
    if (c) return c;
    c = new JClass(PRIM[d]);
    c.isPrimitive = true;
    c.desc = d;
    c.state = "initialized";
    c.access = ACC.PUBLIC | ACC.FINAL | ACC.ABSTRACT;
    this.classes.set(key, c);
    return c;
  }

  classByDesc(d) {
    if (d[0] === "L") return this.loadClass(d.slice(1, -1));
    if (d[0] === "[") return this.arrayClass(d);
    return this.primClass(d);
  }

  // Load or throw NoClassDefFoundError into Java.
  need(t, name) {
    const c = this.loadClass(name);
    if (!c) this.throwNew(t, "java/lang/NoClassDefFoundError", name.replace(/\//g, "."));
    return c;
  }

  classObject(c) {
    if (c.classObj) return c.classObj;
    const cc = this.loadClass("java/lang/Class");
    const o = new JObject(cc);
    o.$cls = c;
    c.classObj = o;
    return o;
  }

  isSubclass(c, s) {
    for (let k = c; k; k = k.superClass) if (k === s) return true;
    return false;
  }

  implementsIface(c, i) {
    for (let k = c; k; k = k.superClass) {
      for (const x of k.interfaces) if (x === i || this.implementsIface(x, i)) return true;
    }
    return false;
  }

  isAssignable(from, to) {
    if (from === to) return true;
    if (from.isPrimitive || to.isPrimitive) return false;
    if (from.isArray) {
      if (to.isArray) {
        const a = from.component,
          b = to.component;
        if (a.isPrimitive || b.isPrimitive) return a === b;
        return this.isAssignable(a, b);
      }
      return (
        to.name === "java/lang/Object" ||
        to.name === "java/lang/Cloneable" ||
        to.name === "java/io/Serializable"
      );
    }
    if (to.access & ACC.INTERFACE) return this.implementsIface(from, to);
    return this.isSubclass(from, to);
  }

  findMethod(c, nd) {
    for (let k = c; k; k = k.superClass) {
      const m = k.methods.get(nd);
      if (m) return m;
    }
    return this.findIfaceMethod(c, nd);
  }

  findIfaceMethod(c, nd) {
    for (let k = c; k; k = k.superClass) {
      for (const i of k.interfaces) {
        const m = i.methods.get(nd);
        if (m && !(m.access & ACC.ABSTRACT)) return m;
        const r = this.findIfaceMethod(i, nd);
        if (r) return r;
      }
    }
    // abstract declarations still resolve (for invokeinterface on abstract)
    for (let k = c; k; k = k.superClass) {
      for (const i of k.interfaces) {
        const m = i.methods.get(nd);
        if (m) return m;
      }
    }
    return null;
  }

  virtualMethod(c, nd) {
    let m = c.vcache.get(nd);
    if (m !== undefined) return m;
    m = null;
    for (let k = c; k; k = k.superClass) {
      const x = k.methods.get(nd);
      if (x && !(x.access & ACC.STATIC)) {
        m = x;
        break;
      }
    }
    if (!m || m.access & ACC.ABSTRACT) m = this.findIfaceMethod(c, nd) || m;
    c.vcache.set(nd, m);
    return m;
  }

  findField(c, name) {
    for (let k = c; k; k = k.superClass) {
      const f = k.fields.get(name);
      if (f) return f;
      for (const i of k.interfaces) {
        const r = this.findField(i, name);
        if (r) return r;
      }
    }
    return null;
  }

  nativeFor(m) {
    if (m.native === undefined) {
      const key = m.cls.name + "." + m.name + m.desc;
      m.native = this.natives[key] || null;
      if (!m.native && m.access & ACC.NATIVE) m.native = null;
    }
    return m.native;
  }

  str(s) {
    if (s === null || s === undefined) return null;
    const c = this.loadClass("java/lang/String");
    const o = new JObject(c);
    const a = new Uint16Array(s.length);
    for (let i = 0; i < s.length; i++) a[i] = s.charCodeAt(i);
    o.value = new JArray(this.arrayClass("[C"), a);
    return o;
  }

  intern(s) {
    let o = this.interned.get(s);
    if (!o) {
      o = this.str(s);
      this.interned.set(s, o);
    }
    return o;
  }

  jsstr(o) {
    if (!o) return null;
    const a = o.value.a;
    let s = "";
    for (let i = 0; i < a.length; i += 8192)
      s += String.fromCharCode.apply(null, a.subarray(i, i + 8192));
    return s;
  }

  newArray(desc, n) {
    const c = this.arrayClass(desc);
    return new JArray(c, newTyped(desc[1], n));
  }

  hash(o) {
    if (o.$hash === undefined)
      o.$hash = (this.nextHash = (Math.imul(this.nextHash, 1103515245) + 12345) | 0) & 0x7fffffff;
    return o.$hash;
  }

  // Create a Throwable synchronously (constructors of VM exceptions never block).
  newThrowable(t, cname, msg) {
    const c = this.loadClass(cname);
    const o = new JObject(c);
    const m =
      msg == null ? c.methods.get("<init>()V") : c.methods.get("<init>(Ljava/lang/String;)V");
    if (!this.ensureInitSync(t, c)) return o;
    this.invokeSync(t, m, msg == null ? [o] : [o, this.str(String(msg))]);
    return o;
  }

  throwNew(t, cname, msg) {
    throw new JavaThrow(this.newThrowable(t, cname, msg));
  }

  pushFrame(t, m, args) {
    const code = m.code;
    const locals = new Array(code.maxLocals);
    for (let i = 0; i < args.length; i++) locals[i] = args[i];
    // every field a frame ever gets is declared here, so all frames share one hidden class
    const f = {
      m,
      cp: m.cls.cf.cp,
      code: code.code,
      pc: 0,
      locals,
      stack: [],
      sync: null,
      onReturn: null,
      boundary: false,
      marker: false,
      callPc: 0,
      retDesc: undefined,
      clinitOf: null,
    };
    t.frames.push(f);
    return f;
  }

  // Run m to completion on thread t, nested inside the current JS call. The callee must not block.
  invokeSync(t, m, args) {
    const nat = this.nativeFor(m);
    if (nat || !m.code) {
      if (!nat) this.unsatisfied(t, m);
      const r = nat(t, ...this.stripTops(m, args));
      if (r === BLOCK || r === CALLED)
        throw new Error(`invokeSync: ${m.cls.name}.${m.name} blocked`);
      return r;
    }
    const depth = t.frames.length;
    const f = this.pushFrame(t, m, this.withTops(m, args));
    f.boundary = true;
    let result;
    f.onReturn = (v) => {
      result = v;
      return undefined;
    };
    const prev = this.current;
    this.current = t;
    try {
      while (t.frames.length > depth) {
        if (t.state !== "runnable")
          throw new Error(`invokeSync: thread blocked in ${this.where(t)}`);
        this.interpret(t, 100000, depth);
      }
    } finally {
      this.current = prev;
    }
    return result;
  }

  // args given without TOP placeholders -> slot layout
  withTops(m, args) {
    const out = [];
    let k = 0;
    if (!(m.access & ACC.STATIC)) out.push(args[k++]);
    for (const a of m.args) {
      out.push(args[k++]);
      if (a === "J" || a === "D") out.push(TOP);
    }
    return out;
  }

  stripTops(m, slots) {
    const out = [];
    let k = 0;
    if (!(m.access & ACC.STATIC)) out.push(slots[k++]);
    for (const a of m.args) {
      out.push(slots[k++]);
      if (a === "J" || a === "D") k++;
    }
    return out;
  }

  // Push a Java call from inside a native; its result becomes the native's result.
  callFromNative(t, m, args, map = (v) => v) {
    const nat = this.nativeFor(m);
    if (nat || !m.code) {
      if (!nat) this.unsatisfied(t, m);
      return map(nat(t, ...args));
    }
    const f = this.pushFrame(t, m, this.withTops(m, args));
    f.onReturn = map;
    if (m.access & ACC.SYNCHRONIZED) {
      const lock = m.access & ACC.STATIC ? this.classObject(m.cls) : args[0];
      if (!this.tryEnter(t, lock)) {
        // rare; fall back to synchronous acquisition semantics by spinning later
        t.frames.pop();
        throw new Error("callFromNative on contended synchronized method");
      }
      f.sync = lock;
    }
    return CALLED;
  }

  unsatisfied(t, m) {
    const key = m.cls.name + "." + m.name + m.desc;
    if (!this.missingNatives.has(key)) {
      this.missingNatives.add(key);
      this.log(`[jvm] missing native ${key}`);
    }
    this.throwNew(t, "java/lang/UnsatisfiedLinkError", key);
  }

  newThread(jthread, name) {
    const t = new Thread(this, jthread, name);
    this.threads.push(t);
    return t;
  }

  mon(o) {
    return o.$mon || (o.$mon = { owner: null, count: 0, waiters: [] });
  }

  tryEnter(t, o) {
    const m = this.mon(o);
    if (m.owner === null || m.owner === t) {
      m.owner = t;
      m.count++;
      return true;
    }
    return false;
  }

  exit(t, o) {
    const m = this.mon(o);
    if (m.owner !== t) this.throwNew(t, "java/lang/IllegalMonitorStateException", null);
    if (--m.count === 0) m.owner = null;
  }

  wait(t, o, ms) {
    const m = this.mon(o);
    if (m.owner !== t) this.throwNew(t, "java/lang/IllegalMonitorStateException", null);
    if (t.interrupted) {
      t.interrupted = false;
      this.throwNew(t, "java/lang/InterruptedException", null);
    }
    const count = m.count;
    m.owner = null;
    m.count = 0;
    m.waiters.push(t);
    t.state = "waiting";
    t.waitingOn = o;
    t.wakeAt = ms > 0 ? this.now() + ms : Infinity;
    t.notified = false;
    t.resume = () => {
      // woken by notify, timeout or interrupt: reacquire the monitor first
      const i = m.waiters.indexOf(t);
      if (i >= 0) m.waiters.splice(i, 1);
      if (m.owner !== null && m.owner !== t) {
        t.state = "blocked";
        t.blockedOn = o;
        return BLOCK;
      }
      m.owner = t;
      m.count = count;
      t.waitingOn = null;
      if (t.interrupted) {
        t.interrupted = false;
        this.throwNew(t, "java/lang/InterruptedException", null);
      }
      return undefined;
    };
    return BLOCK;
  }

  notify(t, o, all) {
    const m = this.mon(o);
    if (m.owner !== t) this.throwNew(t, "java/lang/IllegalMonitorStateException", null);
    const ws = all ? m.waiters.splice(0) : m.waiters.splice(0, 1);
    for (const w of ws) {
      w.state = "blocked"; // will try to reacquire via resume()
      w.blockedOn = o;
      w.wakeAt = Infinity;
    }
  }

  sleep(t, ms) {
    t.state = "sleeping";
    t.wakeAt = this.now() + Math.max(0, ms);
    t.resume = () => {
      if (t.interrupted) {
        t.interrupted = false;
        this.throwNew(t, "java/lang/InterruptedException", null);
      }
      return undefined;
    };
    return BLOCK;
  }

  interrupt(t) {
    t.interrupted = true;
    if (t.state === "sleeping") t.wakeAt = 0;
    if (t.state === "waiting") {
      const m = this.mon(t.waitingOn);
      const i = m.waiters.indexOf(t);
      if (i >= 0) m.waiters.splice(i, 1);
      t.state = "blocked";
      t.blockedOn = t.waitingOn;
    }
  }

  now() {
    return performance.now();
  }

  // Can t run now? Wakes sleepers/waiters whose time has come.
  ready(t, now) {
    switch (t.state) {
      case "runnable":
        return true;
      case "sleeping":
      case "waiting":
        if (now >= t.wakeAt) {
          t.state = "blocked";
          t.blockedOn = t.waitingOn || null;
          return true;
        }
        return false;
      case "blocked": {
        const o = t.blockedOn;
        if (!o) return true;
        const m = this.mon(o);
        return m.owner === null || m.owner === t;
      }
      default:
        return false;
    }
  }

  // Run one scheduling slice. Returns ms until something can run (0 = immediately, Infinity = idle).
  runSlice(budgetMs = 20) {
    const end = this.now() + budgetMs;
    let ran = true;
    while (ran && this.now() < end && !this.exited) {
      ran = false;
      for (const t of this.threads.slice()) {
        if (t.state === "terminated" || this.exited) continue;
        if (!this.ready(t, this.now())) continue;
        ran = true;
        this.current = t;
        if (t.state !== "runnable") {
          t.state = "runnable";
          if (t.resume) {
            const r = this.runResume(t);
            if (r === BLOCK) continue;
          }
        }
        this.interpret(t, 20000);
        if (t.state === "runnable" && !t.frames.length) this.terminate(t);
        this.current = null;
      }
      this.threads = this.threads.filter((t) => t.state !== "terminated");
    }
    if (this.exited || !this.threads.length) return Infinity;
    const now = this.now();
    let next = Infinity;
    for (const t of this.threads) {
      if (this.ready(t, now)) return 0;
      if (t.state === "sleeping" || t.state === "waiting") next = Math.min(next, t.wakeAt - now);
    }
    // While booting, nothing is waiting on real time: wake the next timed sleeper now
    // instead of idling (the clock itself is untouched).
    if (this.fastForward && next > 0 && next !== Infinity) {
      for (const t of this.threads)
        if ((t.state === "sleeping" || t.state === "waiting") && t.wakeAt !== Infinity)
          t.wakeAt -= next;
      return 0;
    }
    return next;
  }

  runResume(t) {
    const fn = t.resume;
    t.resume = null;
    let r;
    try {
      r = fn();
    } catch (e) {
      if (e instanceof JavaThrow) {
        this.raise(t, e.obj, 0, true);
        return undefined;
      }
      throw e;
    }
    if (r === BLOCK) {
      if (!t.resume) t.resume = fn;
      return BLOCK;
    }
    const f = t.frames[t.frames.length - 1];
    if (f && t.pendingRet) this.pushRet(f, t.pendingRet, r);
    t.pendingRet = null;
    return r;
  }

  pushRet(f, ret, v) {
    if (ret === "V") return;
    f.stack.push(v);
    if (ret === "J" || ret === "D") f.stack.push(TOP);
  }

  where(t) {
    const f = t.frames[t.frames.length - 1];
    return f ? `${f.m.cls.name}.${f.m.name}${f.m.desc} pc=${f.pc}` : "(no frame)";
  }

  // Returns true if c is initialized (or being initialized by t). Otherwise pushes
  // <clinit> frames and returns false: the caller must re-execute its instruction.
  ensureInit(t, c) {
    if (c.state === "initialized") return true;
    if (c.state === "initializing") {
      if (c.initThread === t) return true;
      t.state = "blocked";
      t.blockedOn = null;
      t.resume = () => (c.state === "initialized" ? undefined : BLOCK);
      return false;
    }
    c.state = "initializing";
    c.initThread = t;
    const clinit = c.methods.get("<clinit>()V");
    const done = () => {
      c.state = "initialized";
      c.initThread = null;
      return undefined;
    };
    if (clinit) {
      const f = this.pushFrame(t, clinit, []);
      f.onReturn = done;
      f.clinitOf = c;
    } else {
      // no <clinit>: mark done once superclasses finish (a marker frame)
      t.frames.push({
        m: null,
        cp: null,
        code: null,
        pc: 0,
        locals: null,
        stack: [],
        sync: null,
        onReturn: done,
        boundary: false,
        marker: true,
        callPc: 0,
        retDesc: undefined,
        clinitOf: null,
      });
    }
    if (c.superClass && c.superClass.state !== "initialized") this.ensureInit(t, c.superClass);
    return false;
  }

  ensureInitSync(t, c) {
    if (c.state === "initialized" || (c.state === "initializing" && c.initThread === t))
      return true;
    const depth = t.frames.length;
    if (this.ensureInit(t, c)) return true;
    const prev = this.current;
    this.current = t;
    try {
      while (t.frames.length > depth) {
        if (t.state !== "runnable")
          throw new Error(`class init of ${c.name} blocked in ${this.where(t)}`);
        this.interpret(t, 100000, depth);
      }
    } finally {
      this.current = prev;
    }
    return c.state === "initialized";
  }

  // Unwind t's frames to a handler for exception object ex.
  // topIsCaller: the top frame is suspended at a return address (exception from a resumed
  // native), so its call-site pc decides the handler, as for every frame below it.
  raise(t, ex, floor = 0, topIsCaller = false) {
    let caller = topIsCaller;
    while (t.frames.length > floor) {
      const f = t.frames[t.frames.length - 1];
      if (f.marker) {
        t.frames.pop();
        caller = true;
        continue;
      }
      const pc = caller ? f.callPc : f.pc;
      for (const h of f.m.code.handlers) {
        if (pc >= h.start && pc < h.end) {
          if (h.type) {
            const hc = this.loadClass(f.cp[h.type].name);
            if (!hc || !this.isAssignable(ex.cls, hc)) continue;
          }
          f.stack.length = 0;
          f.stack.push(ex);
          f.pc = h.handler;
          return true;
        }
      }
      t.frames.pop();
      caller = true;
      if (f.sync) this.exitQuiet(t, f.sync);
      if (f.clinitOf) {
        f.clinitOf.state = "error";
        f.clinitOf.initThread = null;
      }
      if (f.boundary) throw new JavaThrow(ex);
    }
    this.uncaught(t, ex);
    return false;
  }

  exitQuiet(t, o) {
    const m = this.mon(o);
    if (m.owner === t && --m.count === 0) m.owner = null;
  }

  uncaught(t, ex) {
    const msg = this.describe(ex);
    this.log(`[jvm] uncaught in ${t.name}: ${msg}`);
    if (ex.$trace)
      for (const e of ex.$trace.slice(0, 25))
        this.log(`        at ${e.cls}.${e.method}(${e.file}:${e.line})`);
    this.terminate(t);
  }

  describe(ex) {
    let s = ex.cls.name.replace(/\//g, ".");
    const msg = ex.detailMessage;
    if (msg) s += ": " + this.jsstr(msg);
    if (ex.cause && ex.cause !== ex) s += " / caused by " + this.describe(ex.cause);
    return s;
  }

  terminate(t) {
    t.state = "terminated";
    t.frames.length = 0;
    if (t.onExit) t.onExit();
    if (t.jthread) {
      // wake joiners
      const m = this.mon(t.jthread);
      for (const w of m.waiters.splice(0)) {
        w.state = "blocked";
        w.blockedOn = t.jthread;
      }
    }
  }

  // Stack trace snapshot for Throwable.fillInStackTrace.
  trace(t) {
    const out = [];
    for (let i = t.frames.length - 1; i >= 0; i--) {
      const f = t.frames[i];
      if (f.marker) continue;
      let line = -1;
      for (const [start, ln] of f.m.code.lines) if (start <= f.pc) line = ln;
      out.push({
        cls: f.m.cls.name.replace(/\//g, "."),
        method: f.m.name,
        file: f.m.cls.cf.sourceFile,
        line,
      });
    }
    return out;
  }

  interpret(t, budget, floor = 0) {
    const frames = t.frames;
    while (budget > 0 && t.state === "runnable" && frames.length > floor) {
      const f = frames[frames.length - 1];
      if (f.marker) {
        frames.pop();
        f.onReturn();
        budget--;
        continue;
      }
      try {
        budget = this.step(t, f, budget);
      } catch (e) {
        if (e instanceof JavaThrow) {
          budget--;
          this.raise(t, e.obj, 0);
          continue;
        }
        if (e instanceof RangeError || e instanceof TypeError) {
          this.log(`[jvm] internal error in ${this.where(t)}: ${e.stack}`);
        }
        throw e;
      }
    }
  }

  // Return from frame f with value v (undefined for void).
  ret(t, f, v) {
    t.frames.pop();
    if (f.sync) this.exit(t, f.sync);
    const caller = t.frames[t.frames.length - 1];
    if (f.onReturn) {
      const r = f.onReturn(v);
      // a frame pushed by a native: its mapped value is the native's result
      if (f.retDesc !== undefined && caller) this.pushRet(caller, f.retDesc, r);
      return;
    }
    if (caller && v !== undefined) this.pushRet(caller, f.m.ret, v);
  }

  // Invoke m with slot-layout args already popped. Returns false if the caller must retry.
  invoke(t, m, args) {
    if (m.access & ACC.SYNCHRONIZED) {
      const lock = m.access & ACC.STATIC ? this.classObject(m.cls) : args[0];
      if (!this.tryEnter(t, lock)) {
        t.state = "blocked";
        t.blockedOn = lock;
        return false;
      }
      return this.invokeLocked(t, m, args, lock);
    }
    return this.invokeLocked(t, m, args, null);
  }

  invokeLocked(t, m, args, lock) {
    const nat = this.nativeFor(m);
    if (nat || !m.code) {
      if (!nat) {
        if (lock) this.exitQuiet(t, lock);
        if (m.access & ACC.ABSTRACT)
          this.throwNew(t, "java/lang/AbstractMethodError", m.cls.name + "." + m.name);
        this.unsatisfied(t, m);
      }
      const caller = t.frames[t.frames.length - 1];
      const depth = t.frames.length;
      let r;
      try {
        r = nat(t, ...this.stripTops(m, args));
      } finally {
        if (lock && r !== CALLED) this.exitQuiet(t, lock);
      }
      if (r === BLOCK) {
        t.pendingRet = m.ret;
        return true;
      }
      if (r === CALLED) {
        // the native pushed a frame; its mapped return value goes to the caller
        const top = t.frames[t.frames.length - 1];
        if (t.frames.length > depth) {
          top.retDesc = m.ret;
          if (lock) top.sync = top.sync || lock;
        }
        return true;
      }
      this.pushRet(caller, m.ret, r);
      return true;
    }
    const f = this.pushFrame(t, m, args);
    f.sync = lock;
    return true;
  }

  // Runs frame f until it calls, returns, blocks or uses up the budget; returns what is left.
  npe(t, f, op, start) {
    if (this.debugNPE) {
      const code = f.code;
      const ref = op >= 0xb2 && op <= 0xb9 ? f.cp[(code[start + 1] << 8) | code[start + 2]] : null;
      this.log(
        `[npe] ${f.m.cls.name}.${f.m.name}${f.m.desc} pc=${start} op=0x${op.toString(16)} ${ref ? ref.className + "." + ref.name + ref.desc : ""}`,
      );
    }
    this.throwNew(t, "java/lang/NullPointerException", null);
  }

  // Bounds-checked element array of JArray a.
  arr(t, a, i) {
    if (a === null) this.throwNew(t, "java/lang/NullPointerException", null);
    if (i < 0 || i >= a.a.length)
      this.throwNew(t, "java/lang/ArrayIndexOutOfBoundsException", String(i));
    return a.a;
  }

  step(t, f, budget) {
    const code = f.code;
    const stack = f.stack;
    const locals = f.locals;
    const cp = f.cp;
    let start = f.pc;
    let pc = start;
    let op = 0;
    let v, a, b, i, c;

    try {
      for (;;) {
        start = pc;
        op = code[pc++];
        switch (op) {
          case 0x00:
            break; // nop
          case 0x01:
            stack.push(null);
            break;
          case 0x02:
          case 0x03:
          case 0x04:
          case 0x05:
          case 0x06:
          case 0x07:
          case 0x08:
            stack.push(op - 3);
            break; // iconst_m1..5
          case 0x09:
          case 0x0a:
            stack.push(BigInt(op - 9), TOP);
            break;
          case 0x0b:
          case 0x0c:
          case 0x0d:
            stack.push(op - 0x0b);
            break; // fconst
          case 0x0e:
          case 0x0f:
            stack.push(op - 0x0e, TOP);
            break; // dconst
          case 0x10:
            stack.push((code[pc++] << 24) >> 24);
            break; // bipush
          case 0x11:
            stack.push(((pc += 2), ((code[pc - 2] << 24) >> 16) | code[pc - 1]));
            break; // sipush
          case 0x12:
          case 0x13:
          case 0x14: {
            // ldc, ldc_w, ldc2_w
            i = op === 0x12 ? code[pc++] : ((pc += 2), (code[pc - 2] << 8) | code[pc - 1]);
            const k = cp[i];
            if (k.tag === 8) stack.push(this.intern(k.string));
            else if (k.tag === 7) stack.push(this.classObject(this.need(t, k.name)));
            else if (k.tag === 5 || k.tag === 6) stack.push(k.value, TOP);
            else stack.push(k.value);
            break;
          }
          case 0x15:
          case 0x17:
          case 0x19:
            stack.push(locals[code[pc++]]);
            break; // iload fload aload
          case 0x16:
          case 0x18:
            stack.push(locals[code[pc++]], TOP);
            break; // lload dload
          case 0x1a:
          case 0x1b:
          case 0x1c:
          case 0x1d:
            stack.push(locals[op - 0x1a]);
            break;
          case 0x1e:
          case 0x1f:
          case 0x20:
          case 0x21:
            stack.push(locals[op - 0x1e], TOP);
            break;
          case 0x22:
          case 0x23:
          case 0x24:
          case 0x25:
            stack.push(locals[op - 0x22]);
            break;
          case 0x26:
          case 0x27:
          case 0x28:
          case 0x29:
            stack.push(locals[op - 0x26], TOP);
            break;
          case 0x2a:
          case 0x2b:
          case 0x2c:
          case 0x2d:
            stack.push(locals[op - 0x2a]);
            break;
          case 0x2e:
          case 0x30:
          case 0x32:
          case 0x33:
          case 0x34:
          case 0x35: // iaload faload aaload baload caload saload
            i = stack.pop();
            a = stack.pop();
            stack.push(this.arr(t, a, i)[i]);
            break;
          case 0x2f:
          case 0x31: // laload daload
            i = stack.pop();
            a = stack.pop();
            stack.push(this.arr(t, a, i)[i], TOP);
            break;
          case 0x36:
          case 0x38:
          case 0x3a:
            locals[code[pc++]] = stack.pop();
            break;
          case 0x37:
          case 0x39:
            stack.pop();
            locals[code[pc++]] = stack.pop();
            break;
          case 0x3b:
          case 0x3c:
          case 0x3d:
          case 0x3e:
            locals[op - 0x3b] = stack.pop();
            break;
          case 0x3f:
          case 0x40:
          case 0x41:
          case 0x42:
            stack.pop();
            locals[op - 0x3f] = stack.pop();
            break;
          case 0x43:
          case 0x44:
          case 0x45:
          case 0x46:
            locals[op - 0x43] = stack.pop();
            break;
          case 0x47:
          case 0x48:
          case 0x49:
          case 0x4a:
            stack.pop();
            locals[op - 0x47] = stack.pop();
            break;
          case 0x4b:
          case 0x4c:
          case 0x4d:
          case 0x4e:
            locals[op - 0x4b] = stack.pop();
            break;
          case 0x4f:
          case 0x51:
          case 0x54:
          case 0x55:
          case 0x56: // iastore fastore bastore castore sastore
            v = stack.pop();
            i = stack.pop();
            a = stack.pop();
            this.arr(t, a, i)[i] = v;
            break;
          case 0x50:
          case 0x52: // lastore dastore
            stack.pop();
            v = stack.pop();
            i = stack.pop();
            a = stack.pop();
            this.arr(t, a, i)[i] = v;
            break;
          case 0x53: // aastore
            v = stack.pop();
            i = stack.pop();
            a = stack.pop();
            this.arr(t, a, i);
            if (v !== null && !this.isAssignable(v.cls, a.cls.component))
              this.throwNew(t, "java/lang/ArrayStoreException", v.cls.name);
            a.a[i] = v;
            break;
          case 0x57:
            stack.pop();
            break;
          case 0x58:
            stack.pop();
            stack.pop();
            break;
          case 0x59:
            stack.push(stack[stack.length - 1]);
            break;
          case 0x5a: {
            const v1 = stack.pop(),
              v2 = stack.pop();
            stack.push(v1, v2, v1);
            break;
          }
          case 0x5b: {
            const v1 = stack.pop(),
              v2 = stack.pop(),
              v3 = stack.pop();
            stack.push(v1, v3, v2, v1);
            break;
          }
          case 0x5c: {
            const n = stack.length;
            stack.push(stack[n - 2], stack[n - 1]);
            break;
          }
          case 0x5d: {
            const v1 = stack.pop(),
              v2 = stack.pop(),
              v3 = stack.pop();
            stack.push(v2, v1, v3, v2, v1);
            break;
          }
          case 0x5e: {
            const v1 = stack.pop(),
              v2 = stack.pop(),
              v3 = stack.pop(),
              v4 = stack.pop();
            stack.push(v2, v1, v4, v3, v2, v1);
            break;
          }
          case 0x5f: {
            const v1 = stack.pop(),
              v2 = stack.pop();
            stack.push(v1, v2);
            break;
          }
          case 0x60:
            b = stack.pop();
            a = stack.pop();
            stack.push((a + b) | 0);
            break;
          case 0x61:
            stack.pop();
            b = stack.pop();
            stack.pop();
            a = stack.pop();
            stack.push(BigInt.asIntN(64, a + b), TOP);
            break;
          case 0x62:
            b = stack.pop();
            a = stack.pop();
            stack.push(Math.fround(a + b));
            break;
          case 0x63:
            stack.pop();
            b = stack.pop();
            stack.pop();
            a = stack.pop();
            stack.push(a + b, TOP);
            break;
          case 0x64:
            b = stack.pop();
            a = stack.pop();
            stack.push((a - b) | 0);
            break;
          case 0x65:
            stack.pop();
            b = stack.pop();
            stack.pop();
            a = stack.pop();
            stack.push(BigInt.asIntN(64, a - b), TOP);
            break;
          case 0x66:
            b = stack.pop();
            a = stack.pop();
            stack.push(Math.fround(a - b));
            break;
          case 0x67:
            stack.pop();
            b = stack.pop();
            stack.pop();
            a = stack.pop();
            stack.push(a - b, TOP);
            break;
          case 0x68:
            b = stack.pop();
            a = stack.pop();
            stack.push(Math.imul(a, b));
            break;
          case 0x69:
            stack.pop();
            b = stack.pop();
            stack.pop();
            a = stack.pop();
            stack.push(BigInt.asIntN(64, a * b), TOP);
            break;
          case 0x6a:
            b = stack.pop();
            a = stack.pop();
            stack.push(Math.fround(a * b));
            break;
          case 0x6b:
            stack.pop();
            b = stack.pop();
            stack.pop();
            a = stack.pop();
            stack.push(a * b, TOP);
            break;
          case 0x6c:
            b = stack.pop();
            a = stack.pop();
            if (b === 0) this.throwNew(t, "java/lang/ArithmeticException", "/ by zero");
            stack.push((a / b) | 0);
            break;
          case 0x6d:
            stack.pop();
            b = stack.pop();
            stack.pop();
            a = stack.pop();
            if (b === 0n) this.throwNew(t, "java/lang/ArithmeticException", "/ by zero");
            stack.push(BigInt.asIntN(64, a / b), TOP);
            break;
          case 0x6e:
            b = stack.pop();
            a = stack.pop();
            stack.push(Math.fround(a / b));
            break;
          case 0x6f:
            stack.pop();
            b = stack.pop();
            stack.pop();
            a = stack.pop();
            stack.push(a / b, TOP);
            break;
          case 0x70:
            b = stack.pop();
            a = stack.pop();
            if (b === 0) this.throwNew(t, "java/lang/ArithmeticException", "/ by zero");
            stack.push((a % b) | 0);
            break;
          case 0x71:
            stack.pop();
            b = stack.pop();
            stack.pop();
            a = stack.pop();
            if (b === 0n) this.throwNew(t, "java/lang/ArithmeticException", "/ by zero");
            stack.push(a % b, TOP);
            break;
          case 0x72:
            b = stack.pop();
            a = stack.pop();
            stack.push(Math.fround(a % b));
            break;
          case 0x73:
            stack.pop();
            b = stack.pop();
            stack.pop();
            a = stack.pop();
            stack.push(a % b, TOP);
            break;
          case 0x74:
            stack.push(-stack.pop() | 0);
            break;
          case 0x75:
            stack.pop();
            stack.push(BigInt.asIntN(64, -stack.pop()), TOP);
            break;
          case 0x76:
            stack.push(-stack.pop());
            break;
          case 0x77:
            stack.pop();
            stack.push(-stack.pop(), TOP);
            break;
          case 0x78:
            b = stack.pop();
            a = stack.pop();
            stack.push(a << b);
            break;
          case 0x79:
            b = stack.pop();
            stack.pop();
            a = stack.pop();
            stack.push(BigInt.asIntN(64, a << BigInt(b & 63)), TOP);
            break;
          case 0x7a:
            b = stack.pop();
            a = stack.pop();
            stack.push(a >> b);
            break;
          case 0x7b:
            b = stack.pop();
            stack.pop();
            a = stack.pop();
            stack.push(a >> BigInt(b & 63), TOP);
            break;
          case 0x7c:
            b = stack.pop();
            a = stack.pop();
            stack.push((a >>> b) | 0);
            break;
          case 0x7d:
            b = stack.pop();
            stack.pop();
            a = stack.pop();
            stack.push(BigInt.asIntN(64, BigInt.asUintN(64, a) >> BigInt(b & 63)), TOP);
            break;
          case 0x7e:
            b = stack.pop();
            a = stack.pop();
            stack.push(a & b);
            break;
          case 0x7f:
            stack.pop();
            b = stack.pop();
            stack.pop();
            a = stack.pop();
            stack.push(a & b, TOP);
            break;
          case 0x80:
            b = stack.pop();
            a = stack.pop();
            stack.push(a | b);
            break;
          case 0x81:
            stack.pop();
            b = stack.pop();
            stack.pop();
            a = stack.pop();
            stack.push(a | b, TOP);
            break;
          case 0x82:
            b = stack.pop();
            a = stack.pop();
            stack.push(a ^ b);
            break;
          case 0x83:
            stack.pop();
            b = stack.pop();
            stack.pop();
            a = stack.pop();
            stack.push(a ^ b, TOP);
            break;
          case 0x84:
            i = code[pc++];
            locals[i] = (locals[i] + ((code[pc++] << 24) >> 24)) | 0;
            break; // iinc
          case 0x85:
            stack.push(BigInt(stack.pop()), TOP);
            break; // i2l
          case 0x86:
            stack.push(Math.fround(stack.pop()));
            break; // i2f
          case 0x87:
            stack.push(stack.pop(), TOP);
            break; // i2d
          case 0x88:
            stack.pop();
            stack.push(Number(BigInt.asIntN(32, stack.pop())));
            break; // l2i
          case 0x89:
            stack.pop();
            stack.push(Math.fround(Number(stack.pop())));
            break; // l2f
          case 0x8a:
            stack.pop();
            stack.push(Number(stack.pop()), TOP);
            break; // l2d
          case 0x8b:
            stack.push(f2i(stack.pop()));
            break; // f2i
          case 0x8c:
            stack.push(f2l(stack.pop()), TOP);
            break; // f2l
          case 0x8d:
            stack.push(stack.pop(), TOP);
            break; // f2d
          case 0x8e:
            stack.pop();
            stack.push(f2i(stack.pop()));
            break; // d2i
          case 0x8f:
            stack.pop();
            stack.push(f2l(stack.pop()), TOP);
            break; // d2l
          case 0x90:
            stack.pop();
            stack.push(Math.fround(stack.pop()));
            break; // d2f
          case 0x91:
            stack.push((stack.pop() << 24) >> 24);
            break;
          case 0x92:
            stack.push(stack.pop() & 0xffff);
            break;
          case 0x93:
            stack.push((stack.pop() << 16) >> 16);
            break;
          case 0x94:
            stack.pop();
            b = stack.pop();
            stack.pop();
            a = stack.pop();
            stack.push(a < b ? -1 : a > b ? 1 : 0);
            break;
          case 0x95:
          case 0x96:
            b = stack.pop();
            a = stack.pop();
            stack.push(cmp(a, b, op === 0x96 ? 1 : -1));
            break;
          case 0x97:
          case 0x98:
            stack.pop();
            b = stack.pop();
            stack.pop();
            a = stack.pop();
            stack.push(cmp(a, b, op === 0x98 ? 1 : -1));
            break;
          case 0x99:
            v = stack.pop();
            i = ((pc += 2), ((code[pc - 2] << 24) >> 16) | code[pc - 1]);
            if (v === 0) pc = start + i;
            break;
          case 0x9a:
            v = stack.pop();
            i = ((pc += 2), ((code[pc - 2] << 24) >> 16) | code[pc - 1]);
            if (v !== 0) pc = start + i;
            break;
          case 0x9b:
            v = stack.pop();
            i = ((pc += 2), ((code[pc - 2] << 24) >> 16) | code[pc - 1]);
            if (v < 0) pc = start + i;
            break;
          case 0x9c:
            v = stack.pop();
            i = ((pc += 2), ((code[pc - 2] << 24) >> 16) | code[pc - 1]);
            if (v >= 0) pc = start + i;
            break;
          case 0x9d:
            v = stack.pop();
            i = ((pc += 2), ((code[pc - 2] << 24) >> 16) | code[pc - 1]);
            if (v > 0) pc = start + i;
            break;
          case 0x9e:
            v = stack.pop();
            i = ((pc += 2), ((code[pc - 2] << 24) >> 16) | code[pc - 1]);
            if (v <= 0) pc = start + i;
            break;
          case 0x9f:
          case 0xa5:
            b = stack.pop();
            a = stack.pop();
            i = ((pc += 2), ((code[pc - 2] << 24) >> 16) | code[pc - 1]);
            if (a === b) pc = start + i;
            break;
          case 0xa0:
          case 0xa6:
            b = stack.pop();
            a = stack.pop();
            i = ((pc += 2), ((code[pc - 2] << 24) >> 16) | code[pc - 1]);
            if (a !== b) pc = start + i;
            break;
          case 0xa1:
            b = stack.pop();
            a = stack.pop();
            i = ((pc += 2), ((code[pc - 2] << 24) >> 16) | code[pc - 1]);
            if (a < b) pc = start + i;
            break;
          case 0xa2:
            b = stack.pop();
            a = stack.pop();
            i = ((pc += 2), ((code[pc - 2] << 24) >> 16) | code[pc - 1]);
            if (a >= b) pc = start + i;
            break;
          case 0xa3:
            b = stack.pop();
            a = stack.pop();
            i = ((pc += 2), ((code[pc - 2] << 24) >> 16) | code[pc - 1]);
            if (a > b) pc = start + i;
            break;
          case 0xa4:
            b = stack.pop();
            a = stack.pop();
            i = ((pc += 2), ((code[pc - 2] << 24) >> 16) | code[pc - 1]);
            if (a <= b) pc = start + i;
            break;
          case 0xa7:
            pc = start + ((pc += 2), ((code[pc - 2] << 24) >> 16) | code[pc - 1]);
            break; // goto
          case 0xa8:
            i = ((pc += 2), ((code[pc - 2] << 24) >> 16) | code[pc - 1]);
            stack.push(pc);
            pc = start + i;
            break; // jsr
          case 0xa9:
            pc = locals[code[pc]];
            break; // ret
          case 0xaa: {
            // tableswitch
            pc = (start + 4) & ~3;
            const def =
                ((pc += 4),
                (code[pc - 4] << 24) | (code[pc - 3] << 16) | (code[pc - 2] << 8) | code[pc - 1]),
              lo =
                ((pc += 4),
                (code[pc - 4] << 24) | (code[pc - 3] << 16) | (code[pc - 2] << 8) | code[pc - 1]),
              hi =
                ((pc += 4),
                (code[pc - 4] << 24) | (code[pc - 3] << 16) | (code[pc - 2] << 8) | code[pc - 1]);
            v = stack.pop();
            if (v < lo || v > hi) pc = start + def;
            else {
              pc += (v - lo) * 4;
              pc =
                start +
                ((pc += 4),
                (code[pc - 4] << 24) | (code[pc - 3] << 16) | (code[pc - 2] << 8) | code[pc - 1]);
            }
            break;
          }
          case 0xab: {
            // lookupswitch
            pc = (start + 4) & ~3;
            const def =
                ((pc += 4),
                (code[pc - 4] << 24) | (code[pc - 3] << 16) | (code[pc - 2] << 8) | code[pc - 1]),
              n =
                ((pc += 4),
                (code[pc - 4] << 24) | (code[pc - 3] << 16) | (code[pc - 2] << 8) | code[pc - 1]);
            v = stack.pop();
            let target = def;
            for (let k = 0; k < n; k++) {
              const key =
                  ((pc += 4),
                  (code[pc - 4] << 24) | (code[pc - 3] << 16) | (code[pc - 2] << 8) | code[pc - 1]),
                off =
                  ((pc += 4),
                  (code[pc - 4] << 24) | (code[pc - 3] << 16) | (code[pc - 2] << 8) | code[pc - 1]);
              if (key === v) {
                target = off;
                break;
              }
            }
            pc = start + target;
            break;
          }
          case 0xac:
          case 0xae:
          case 0xb0:
            this.ret(t, f, stack.pop());
            return budget - 1; // ireturn freturn areturn
          case 0xad:
          case 0xaf:
            stack.pop();
            this.ret(t, f, stack.pop());
            return budget - 1; // lreturn dreturn
          case 0xb1:
            this.ret(t, f, undefined);
            return budget - 1;
          case 0xb2:
          case 0xb3: {
            // getstatic putstatic
            const ref = cp[((pc += 2), (code[pc - 2] << 8) | code[pc - 1])];
            if (!ref.field) {
              const rc = this.need(t, ref.className);
              ref.field = this.findField(rc, ref.name);
              if (!ref.field)
                this.throwNew(t, "java/lang/NoSuchFieldError", ref.className + "." + ref.name);
            }
            const fc = ref.field.cls;
            if (fc.state !== "initialized" && !this.ensureInit(t, fc)) {
              f.pc = f.callPc = start;
              return budget;
            }
            if (op === 0xb2) {
              stack.push(fc.statics[ref.field.key]);
              if (ref.desc === "J" || ref.desc === "D") stack.push(TOP);
            } else {
              if (ref.desc === "J" || ref.desc === "D") stack.pop();
              fc.statics[ref.field.key] = stack.pop();
            }
            break;
          }
          case 0xb4: {
            // getfield
            const ref = cp[((pc += 2), (code[pc - 2] << 8) | code[pc - 1])];
            const key = ref.key || this.fieldKey(t, ref);
            const o = stack.pop();
            if (o === null) this.npe(t, f, op, start);
            stack.push(o[key]);
            if (ref.desc === "J" || ref.desc === "D") stack.push(TOP);
            break;
          }
          case 0xb5: {
            // putfield
            const ref = cp[((pc += 2), (code[pc - 2] << 8) | code[pc - 1])];
            const key = ref.key || this.fieldKey(t, ref);
            if (ref.desc === "J" || ref.desc === "D") stack.pop();
            v = stack.pop();
            const o = stack.pop();
            if (o === null) this.npe(t, f, op, start);
            o[key] = v;
            break;
          }
          case 0xb6:
          case 0xb7:
          case 0xb8:
          case 0xb9: {
            // invokevirtual special static interface
            const ref = cp[((pc += 2), (code[pc - 2] << 8) | code[pc - 1])];
            if (op === 0xb9) pc += 2;
            if (!ref.method) {
              const rc = this.need(t, ref.className);
              const nd = ref.name + ref.desc;
              ref.nd = nd;
              ref.method = rc.isArray
                ? this.findMethod(this.loadClass("java/lang/Object"), nd)
                : this.findMethod(rc, nd);
              if (!ref.method)
                this.throwNew(t, "java/lang/NoSuchMethodError", ref.className + "." + nd);
            }
            let m = ref.method;
            const nslots = m.argSlots;
            if (op === 0xb8) {
              if (m.cls.state !== "initialized" && !this.ensureInit(t, m.cls)) {
                f.pc = f.callPc = start;
                return budget;
              }
            } else {
              const recv = stack[stack.length - nslots];
              if (recv === null) this.npe(t, f, op, start);
              if (op === 0xb7) {
                const cur = f.m.cls;
                if (
                  m.name !== "<init>" &&
                  !(m.access & ACC.PRIVATE) &&
                  cur.superClass &&
                  m.cls !== cur &&
                  this.isSubclass(cur.superClass, m.cls)
                ) {
                  m = this.findMethod(cur.superClass, ref.nd);
                }
              } else if (!(m.access & ACC.PRIVATE)) {
                if (ref.icCls === recv.cls)
                  m = ref.icM; // one-entry inline cache per call site
                else {
                  m = this.virtualMethod(recv.cls, ref.nd);
                  if (!m)
                    this.throwNew(t, "java/lang/AbstractMethodError", recv.cls.name + "." + ref.nd);
                  ref.icCls = recv.cls;
                  ref.icM = m;
                }
              }
            }
            f.pc = start;
            f.callPc = start;
            // fast path: plain bytecode callee; its arguments go straight from our stack to its locals
            const nat = m.native !== undefined ? m.native : this.nativeFor(m);
            if (nat === null && m.code !== null && !(m.access & ACC.SYNCHRONIZED)) {
              const base = stack.length - nslots;
              const nl = new Array(m.code.maxLocals);
              for (let k = 0; k < nslots; k++) nl[k] = stack[base + k];
              stack.length = base;
              f.pc = pc;
              t.frames.push({
                m,
                cp: m.cls.cf.cp,
                code: m.code.code,
                pc: 0,
                locals: nl,
                stack: [],
                sync: null,
                onReturn: null,
                boundary: false,
                marker: false,
                callPc: 0,
                retDesc: undefined,
                clinitOf: null,
              });
              return budget - 1;
            }
            const args = stack.splice(stack.length - nslots, nslots);
            const ok = this.invoke(t, m, args);
            if (!ok) {
              stack.push(...args);
              f.pc = start;
              return budget;
            }
            f.pc = pc;
            // a native that finished synchronously leaves us in the same frame: keep going
            if (t.frames[t.frames.length - 1] === f && t.state === "runnable") break;
            return budget - 1;
          }
          case 0xba:
            this.throwNew(
              t,
              "java/lang/UnsupportedOperationException",
              "invokedynamic in " + f.m.cls.name,
            );
            break;
          case 0xbb: {
            // new
            const ref = cp[((pc += 2), (code[pc - 2] << 8) | code[pc - 1])];
            c = ref.cls || (ref.cls = this.need(t, ref.name));
            if (c.state !== "initialized" && !this.ensureInit(t, c)) {
              f.pc = f.callPc = start;
              return budget;
            }
            if (c.access & (ACC.ABSTRACT | ACC.INTERFACE))
              this.throwNew(t, "java/lang/InstantiationError", c.name);
            stack.push(new JObject(c));
            break;
          }
          case 0xbc: {
            // newarray
            const at = code[pc++];
            const n = stack.pop();
            if (n < 0) this.throwNew(t, "java/lang/NegativeArraySizeException", String(n));
            stack.push(this.newArray("[" + "    ZCFDBSIJ"[at], n));
            break;
          }
          case 0xbd: {
            // anewarray
            const ref = cp[((pc += 2), (code[pc - 2] << 8) | code[pc - 1])];
            const n = stack.pop();
            if (n < 0) this.throwNew(t, "java/lang/NegativeArraySizeException", String(n));
            const en = ref.name;
            stack.push(this.newArray(en[0] === "[" ? "[" + en : "[L" + en + ";", n));
            break;
          }
          case 0xbe:
            a = stack.pop();
            if (a === null) this.npe(t, f, op, start);
            stack.push(a.a.length);
            break;
          case 0xbf:
            v = stack.pop();
            if (v === null) this.npe(t, f, op, start);
            throw new JavaThrow(v);
          case 0xc0:
          case 0xc1: {
            // checkcast instanceof
            const ref = cp[((pc += 2), (code[pc - 2] << 8) | code[pc - 1])];
            c = ref.cls || (ref.cls = this.need(t, ref.name));
            v = stack[stack.length - 1];
            if (op === 0xc0) {
              if (v !== null && !this.isAssignable(v.cls, c))
                this.throwNew(
                  t,
                  "java/lang/ClassCastException",
                  `${v.cls.name.replace(/\//g, ".")} cannot be cast to ${c.name.replace(/\//g, ".")}`,
                );
            } else {
              stack[stack.length - 1] = v !== null && this.isAssignable(v.cls, c) ? 1 : 0;
            }
            break;
          }
          case 0xc2: // monitorenter
            v = stack[stack.length - 1];
            if (v === null) this.npe(t, f, op, start);
            if (!this.tryEnter(t, v)) {
              t.state = "blocked";
              t.blockedOn = v;
              f.pc = start;
              return budget;
            }
            stack.pop();
            break;
          case 0xc3:
            v = stack.pop();
            if (v === null) this.npe(t, f, op, start);
            this.exit(t, v);
            break;
          case 0xc4: {
            // wide
            const wop = code[pc++];
            i = ((pc += 2), (code[pc - 2] << 8) | code[pc - 1]);
            if (wop === 0x84) {
              locals[i] =
                (locals[i] + ((pc += 2), ((code[pc - 2] << 24) >> 16) | code[pc - 1])) | 0;
            } else if (wop >= 0x15 && wop <= 0x19) {
              stack.push(locals[i]);
              if (wop === 0x16 || wop === 0x18) stack.push(TOP);
            } else if (wop >= 0x36 && wop <= 0x3a) {
              if (wop === 0x37 || wop === 0x39) stack.pop();
              locals[i] = stack.pop();
            } else if (wop === 0xa9) {
              pc = locals[i];
            }
            break;
          }
          case 0xc5: {
            // multianewarray
            const ref = cp[((pc += 2), (code[pc - 2] << 8) | code[pc - 1])];
            const dims = code[pc++];
            const counts = stack.splice(stack.length - dims, dims);
            for (const n of counts)
              if (n < 0) this.throwNew(t, "java/lang/NegativeArraySizeException", String(n));
            const mk = (desc, d) => {
              const out = this.newArray(desc, counts[d]);
              if (d + 1 < dims)
                for (let k = 0; k < counts[d]; k++) out.a[k] = mk(desc.slice(1), d + 1);
              return out;
            };
            stack.push(mk(ref.name, 0));
            break;
          }
          case 0xc6:
            v = stack.pop();
            i = ((pc += 2), ((code[pc - 2] << 24) >> 16) | code[pc - 1]);
            if (v === null) pc = start + i;
            break;
          case 0xc7:
            v = stack.pop();
            i = ((pc += 2), ((code[pc - 2] << 24) >> 16) | code[pc - 1]);
            if (v !== null) pc = start + i;
            break;
          case 0xc8:
            pc =
              start +
              ((pc += 4),
              (code[pc - 4] << 24) | (code[pc - 3] << 16) | (code[pc - 2] << 8) | code[pc - 1]);
            break; // goto_w
          case 0xc9:
            i =
              ((pc += 4),
              (code[pc - 4] << 24) | (code[pc - 3] << 16) | (code[pc - 2] << 8) | code[pc - 1]);
            stack.push(pc);
            pc = start + i;
            break; // jsr_w
          default:
            throw new Error(
              `unknown opcode 0x${op.toString(16)} in ${f.m.cls.name}.${f.m.name} at ${start}`,
            );
        }
        if (--budget <= 0) {
          f.pc = pc;
          return 0;
        }
      }
    } catch (e) {
      f.pc = start; // handlers and traces see the faulting instruction
      throw e;
    }
  }

  fieldKey(t, ref) {
    const rc = this.need(t, ref.className);
    const fld = this.findField(rc, ref.name);
    if (!fld) this.throwNew(t, "java/lang/NoSuchFieldError", ref.className + "." + ref.name);
    ref.key = fld.key;
    return ref.key;
  }
}

function f2i(x) {
  if (Number.isNaN(x)) return 0;
  if (x >= 2147483647) return 2147483647;
  if (x <= -2147483648) return -2147483648;
  return Math.trunc(x) | 0;
}

function f2l(x) {
  if (Number.isNaN(x)) return 0n;
  if (x >= 9223372036854775807) return 9223372036854775807n;
  if (x <= -9223372036854775808) return -9223372036854775808n;
  return BigInt(Math.trunc(x));
}

function cmp(a, b, nan) {
  if (Number.isNaN(a) || Number.isNaN(b)) return nan;
  return a < b ? -1 : a > b ? 1 : 0;
}
