// JS implementations of the JDK 8 natives the interpreter needs, plus a virtual file system.
// Keys are "internal/Class.name(desc)". Each native gets (t, ...args) with `this`-less args:
// instance natives receive the receiver first.

import { JObject, JArray, BLOCK, CALLED, JavaThrow } from "./jvm.js";
import { ACC, parseDesc } from "./classfile.js";
import { inflate, Incomplete } from "./inflate.js";

// Paths are absolute, '/'-separated. Directories are tracked explicitly.
export class VFS {
  constructor() {
    this.files = new Map(); // path -> Uint8Array
    this.dirs = new Set(["/"]);
    this.remote = new Map(); // path -> {size, read(pos, len) -> Uint8Array}, fetched on demand
    this.mtime = new Map();
    this.onChange = null;
    this.cwd = "/";
  }
  norm(p) {
    if (!p.startsWith("/")) p = this.cwd + "/" + p;
    const out = [];
    for (const part of p.split("/")) {
      if (!part || part === ".") continue;
      if (part === "..") out.pop();
      else out.push(part);
    }
    return "/" + out.join("/");
  }
  parent(p) {
    const i = p.lastIndexOf("/");
    return i <= 0 ? "/" : p.slice(0, i);
  }
  mkdirs(p) {
    p = this.norm(p);
    while (p !== "/" && !this.dirs.has(p)) {
      this.dirs.add(p);
      p = this.parent(p);
    }
  }
  write(p, data) {
    p = this.norm(p);
    this.mkdirs(this.parent(p));
    this.files.set(p, data);
    this.mtime.set(p, Date.now());
    if (this.onChange) this.onChange(p);
  }
  read(p) {
    p = this.norm(p);
    const d = this.files.get(p);
    if (d) return d;
    const r = this.remote.get(p);
    if (!r) return null;
    const all = r.read(0, r.size); // first write or whole-file read: bring it local
    this.files.set(p, all);
    this.remote.delete(p);
    return all;
  }
  // Mount a file whose bytes are fetched lazily (e.g. a large SD card image over HTTP).
  mount(p, size, read) {
    p = this.norm(p);
    this.mkdirs(this.parent(p));
    this.remote.set(p, { size, read });
  }
  exists(p) {
    p = this.norm(p);
    return this.files.has(p) || this.remote.has(p);
  }
  size(p) {
    p = this.norm(p);
    const d = this.files.get(p);
    if (d) return d.length;
    const r = this.remote.get(p);
    return r ? r.size : 0;
  }
  list(p) {
    p = this.norm(p);
    const pre = p === "/" ? "/" : p + "/";
    const names = new Set();
    for (const k of [...this.files.keys(), ...this.remote.keys(), ...this.dirs]) {
      if (k.startsWith(pre) && k.length > pre.length) names.add(k.slice(pre.length).split("/")[0]);
    }
    return [...names];
  }
}

export function makeNatives(opts = {}) {
  const vfs = opts.vfs || new VFS();
  const stdout = opts.stdout || ((s) => process.stdout.write(s));
  const stderr = opts.stderr || ((s) => process.stderr.write(s));
  const fieldTable = []; // Unsafe offsets -> {key, static, cls}
  const methodTable = []; // reflection slot -> method
  const openFiles = new Map(); // fd -> {path, data, pos, write}
  let nextFd = 3;
  const N = {};
  const vmOf = (t) => t.jvm;
  const noop = () => undefined;

  const reg = (key, fn) => {
    N[key] = fn;
  };
  for (const c of [
    "java/lang/Object",
    "java/lang/Class",
    "java/lang/System",
    "java/lang/Thread",
    "java/lang/ClassLoader",
    "sun/misc/Unsafe",
    "java/lang/Runtime",
  ])
    reg(c + ".registerNatives()V", noop);

  reg("java/lang/Object.getClass()Ljava/lang/Class;", (t, o) => vmOf(t).classObject(o.cls));
  reg("java/lang/Object.hashCode()I", (t, o) => vmOf(t).hash(o));
  reg("java/lang/Object.clone()Ljava/lang/Object;", (t, o) => {
    const vm = vmOf(t);
    if (o instanceof JArray) return new JArray(o.cls, o.a.slice());
    if (!vm.isAssignable(o.cls, vm.loadClass("java/lang/Cloneable")))
      vm.throwNew(t, "java/lang/CloneNotSupportedException", o.cls.name);
    const c = new JObject(o.cls);
    for (const k of Object.keys(o)) if (k !== "$mon" && k !== "$hash") c[k] = o[k];
    return c;
  });
  reg("java/lang/Object.notify()V", (t, o) => vmOf(t).notify(t, o, false));
  reg("java/lang/Object.notifyAll()V", (t, o) => vmOf(t).notify(t, o, true));
  reg("java/lang/Object.wait(J)V", (t, o, ms) => vmOf(t).wait(t, o, Number(ms)));

  const C = (o) => o.$cls;
  reg("java/lang/Class.getPrimitiveClass(Ljava/lang/String;)Ljava/lang/Class;", (t, n) => {
    const vm = vmOf(t);
    const d = {
      boolean: "Z",
      byte: "B",
      char: "C",
      short: "S",
      int: "I",
      long: "J",
      float: "F",
      double: "D",
      void: "V",
    }[vm.jsstr(n)];
    return vm.classObject(vm.primClass(d));
  });
  reg("java/lang/Class.desiredAssertionStatus0(Ljava/lang/Class;)Z", () => 0);
  reg("java/lang/Class.getName0()Ljava/lang/String;", (t, o) =>
    vmOf(t).intern(C(o).name.replace(/\//g, ".")),
  );
  reg("java/lang/Class.isInterface()Z", (t, o) => (C(o).access & ACC.INTERFACE ? 1 : 0));
  reg("java/lang/Class.isArray()Z", (t, o) => (C(o).isArray ? 1 : 0));
  reg("java/lang/Class.isPrimitive()Z", (t, o) => (C(o).isPrimitive ? 1 : 0));
  reg("java/lang/Class.getSuperclass()Ljava/lang/Class;", (t, o) => {
    const c = C(o);
    return c.superClass && !(c.access & ACC.INTERFACE) ? vmOf(t).classObject(c.superClass) : null;
  });
  reg("java/lang/Class.getComponentType()Ljava/lang/Class;", (t, o) =>
    C(o).isArray ? vmOf(t).classObject(C(o).component) : null,
  );
  reg("java/lang/Class.getModifiers()I", (t, o) => C(o).access & 0x7ff);
  reg("java/lang/Class.isInstance(Ljava/lang/Object;)Z", (t, o, x) =>
    x !== null && vmOf(t).isAssignable(x.cls, C(o)) ? 1 : 0,
  );
  reg("java/lang/Class.isAssignableFrom(Ljava/lang/Class;)Z", (t, o, x) =>
    vmOf(t).isAssignable(C(x), C(o)) ? 1 : 0,
  );
  reg("java/lang/Class.getInterfaces0()[Ljava/lang/Class;", (t, o) => {
    const vm = vmOf(t);
    const arr = vm.newArray("[Ljava/lang/Class;", C(o).interfaces.length);
    C(o).interfaces.forEach((i, k) => (arr.a[k] = vm.classObject(i)));
    return arr;
  });
  for (const k of [
    "getEnclosingMethod0()[Ljava/lang/Object;",
    "getDeclaringClass0()Ljava/lang/Class;",
    "getGenericSignature0()Ljava/lang/String;",
    "getRawAnnotations()[B",
    "getRawTypeAnnotations()[B",
    "getConstantPool()Lsun/reflect/ConstantPool;",
    "getProtectionDomain0()Ljava/security/ProtectionDomain;",
    "getSigners()[Ljava/lang/Object;",
  ])
    reg("java/lang/Class." + k, () => null);
  reg(
    "java/lang/Class.forName0(Ljava/lang/String;ZLjava/lang/ClassLoader;Ljava/lang/Class;)Ljava/lang/Class;",
    (t, n, init) => {
      const vm = vmOf(t);
      const name = vm.jsstr(n).replace(/\./g, "/");
      const c = vm.loadClass(name);
      if (!c) vm.throwNew(t, "java/lang/ClassNotFoundException", vm.jsstr(n));
      if (init && c.state !== "initialized") vm.ensureInitSync(t, c);
      return vm.classObject(c);
    },
  );

  const fieldObj = (t, f) => {
    const vm = vmOf(t);
    let slot = fieldTable.findIndex((x) => x.f === f);
    if (slot < 0) {
      slot = fieldTable.length;
      fieldTable.push({ f, key: f.key, static: !!(f.access & ACC.STATIC), cls: f.cls });
    }
    const FC = vm.loadClass("java/lang/reflect/Field");
    vm.ensureInitSync(t, FC);
    const o = new JObject(FC);
    o.clazz = vm.classObject(f.cls);
    o.name = vm.intern(f.name);
    o.type = vm.classObject(vm.classByDesc(f.desc));
    o.modifiers = f.access;
    o.slot = slot;
    return o;
  };
  const execObj = (t, m, cname) => {
    const vm = vmOf(t);
    const EC = vm.loadClass(cname);
    vm.ensureInitSync(t, EC);
    const o = new JObject(EC);
    o.clazz = vm.classObject(m.cls);
    const ptypes = vm.newArray("[Ljava/lang/Class;", m.args.length);
    m.args.forEach((a, i) => (ptypes.a[i] = vm.classObject(vm.classByDesc(a))));
    o.parameterTypes = ptypes;
    o.exceptionTypes = vm.newArray("[Ljava/lang/Class;", 0);
    o.modifiers = m.access;
    let slot = methodTable.indexOf(m);
    if (slot < 0) {
      slot = methodTable.length;
      methodTable.push(m);
    }
    o.slot = slot;
    if (cname === "java/lang/reflect/Method") {
      o.name = vm.intern(m.name);
      o.returnType = vm.classObject(vm.classByDesc(m.ret));
    }
    return o;
  };
  reg("java/lang/Class.getDeclaredFields0(Z)[Ljava/lang/reflect/Field;", (t, o, pub) => {
    const vm = vmOf(t);
    const fs = [...C(o).fields.values()].filter((f) => !pub || f.access & ACC.PUBLIC);
    const arr = vm.newArray("[Ljava/lang/reflect/Field;", fs.length);
    fs.forEach((f, i) => (arr.a[i] = fieldObj(t, f)));
    return arr;
  });
  reg("java/lang/Class.getDeclaredMethods0(Z)[Ljava/lang/reflect/Method;", (t, o, pub) => {
    const vm = vmOf(t);
    const ms = [...C(o).methods.values()].filter(
      (m) => m.name[0] !== "<" && (!pub || m.access & ACC.PUBLIC),
    );
    const arr = vm.newArray("[Ljava/lang/reflect/Method;", ms.length);
    ms.forEach((m, i) => (arr.a[i] = execObj(t, m, "java/lang/reflect/Method")));
    return arr;
  });
  reg(
    "java/lang/Class.getDeclaredConstructors0(Z)[Ljava/lang/reflect/Constructor;",
    (t, o, pub) => {
      const vm = vmOf(t);
      const ms = [...C(o).methods.values()].filter(
        (m) => m.name === "<init>" && (!pub || m.access & ACC.PUBLIC),
      );
      const arr = vm.newArray("[Ljava/lang/reflect/Constructor;", ms.length);
      ms.forEach((m, i) => (arr.a[i] = execObj(t, m, "java/lang/reflect/Constructor")));
      return arr;
    },
  );

  // Boxing helpers for reflection.
  const unbox = (vm, d, v) => {
    if (d.length > 1) return v;
    if (v === null) return d === "J" ? 0n : 0;
    return d === "Z" ||
      d === "B" ||
      d === "C" ||
      d === "S" ||
      d === "I" ||
      d === "J" ||
      d === "F" ||
      d === "D"
      ? v.value
      : v;
  };
  const box = (t, d, v) => {
    const vm = vmOf(t);
    const B = {
      Z: ["java/lang/Boolean", "(Z)Ljava/lang/Boolean;"],
      B: ["java/lang/Byte", "(B)Ljava/lang/Byte;"],
      C: ["java/lang/Character", "(C)Ljava/lang/Character;"],
      S: ["java/lang/Short", "(S)Ljava/lang/Short;"],
      I: ["java/lang/Integer", "(I)Ljava/lang/Integer;"],
      J: ["java/lang/Long", "(J)Ljava/lang/Long;"],
      F: ["java/lang/Float", "(F)Ljava/lang/Float;"],
      D: ["java/lang/Double", "(D)Ljava/lang/Double;"],
    }[d];
    if (!B) return d === "V" ? null : v;
    const c = vm.loadClass(B[0]);
    vm.ensureInitSync(t, c);
    return vm.invokeSync(t, c.methods.get("valueOf" + B[1]), [v]);
  };
  reg(
    "sun/reflect/NativeConstructorAccessorImpl.newInstance0(Ljava/lang/reflect/Constructor;[Ljava/lang/Object;)Ljava/lang/Object;",
    (t, ctor, args) => {
      const vm = vmOf(t);
      const m = methodTable[ctor.slot];
      if (m.cls.state !== "initialized") vm.ensureInitSync(t, m.cls);
      const o = new JObject(m.cls);
      const a = [o, ...m.args.map((d, i) => unbox(vm, d, args ? args.a[i] : null))];
      return vm.callFromNative(t, m, a, () => o);
    },
  );
  reg(
    "sun/reflect/NativeMethodAccessorImpl.invoke0(Ljava/lang/reflect/Method;Ljava/lang/Object;[Ljava/lang/Object;)Ljava/lang/Object;",
    (t, mo, recv, args) => {
      const vm = vmOf(t);
      let m = methodTable[mo.slot];
      const isStatic = m.access & ACC.STATIC;
      if (isStatic && m.cls.state !== "initialized") vm.ensureInitSync(t, m.cls);
      if (!isStatic) {
        if (recv === null) vm.throwNew(t, "java/lang/NullPointerException", null);
        m = vm.virtualMethod(recv.cls, m.name + m.desc) || m;
      }
      const a = m.args.map((d, i) => unbox(vm, d, args ? args.a[i] : null));
      if (!isStatic) a.unshift(recv);
      return vm.callFromNative(t, m, a, (r) => box(t, m.ret, r));
    },
  );

  reg("sun/reflect/Reflection.getCallerClass()Ljava/lang/Class;", (t) => {
    const fs = t.frames.filter((f) => !f.marker);
    // fs[top] is the @CallerSensitive method; its caller is one below (skip reflection frames)
    for (let i = fs.length - 2; i >= 0; i--) {
      const n = fs[i].m.cls.name;
      if (n.startsWith("sun/reflect/") || n === "java/lang/reflect/Method") continue;
      return vmOf(t).classObject(fs[i].m.cls);
    }
    return null;
  });
  reg("sun/reflect/Reflection.getClassAccessFlags(Ljava/lang/Class;)I", (t, o) => C(o).access);
  reg("java/lang/reflect/Array.newArray(Ljava/lang/Class;I)Ljava/lang/Object;", (t, c, n) => {
    const vm = vmOf(t);
    const cc = C(c);
    const d = cc.isPrimitive ? cc.desc : cc.isArray ? cc.name : "L" + cc.name + ";";
    if (n < 0) vm.throwNew(t, "java/lang/NegativeArraySizeException", String(n));
    return vm.newArray("[" + d, n);
  });
  reg("java/lang/reflect/Array.getLength(Ljava/lang/Object;)I", (t, a) => a.a.length);
  reg("java/lang/reflect/Array.get(Ljava/lang/Object;I)Ljava/lang/Object;", (t, a, i) => {
    const e = a.cls.elem;
    return e.length === 1 ? box(t, e, a.a[i]) : a.a[i];
  });
  reg("java/lang/reflect/Array.set(Ljava/lang/Object;ILjava/lang/Object;)V", (t, a, i, v) => {
    const e = a.cls.elem;
    a.a[i] = e.length === 1 ? unbox(vmOf(t), e, v) : v;
  });

  reg("java/lang/System.currentTimeMillis()J", () => BigInt(Date.now()));
  reg("java/lang/System.nanoTime()J", () => BigInt(Math.floor(performance.now() * 1e6)));
  reg("java/lang/System.identityHashCode(Ljava/lang/Object;)I", (t, o) =>
    o === null ? 0 : vmOf(t).hash(o),
  );
  reg(
    "java/lang/System.arraycopy(Ljava/lang/Object;ILjava/lang/Object;II)V",
    (t, src, sp, dst, dp, n) => {
      const vm = vmOf(t);
      if (src === null || dst === null) vm.throwNew(t, "java/lang/NullPointerException", null);
      if (!(src instanceof JArray) || !(dst instanceof JArray))
        vm.throwNew(t, "java/lang/ArrayStoreException", null);
      if (n < 0 || sp < 0 || dp < 0 || sp + n > src.a.length || dp + n > dst.a.length)
        vm.throwNew(t, "java/lang/ArrayIndexOutOfBoundsException", "arraycopy");
      if (src.cls.elem !== dst.cls.elem && (src.cls.elem.length === 1 || dst.cls.elem.length === 1))
        vm.throwNew(t, "java/lang/ArrayStoreException", null);
      if (ArrayBuffer.isView(src.a)) dst.a.set(src.a.subarray(sp, sp + n), dp);
      else if (src === dst && sp < dp)
        for (let i = n - 1; i >= 0; i--) dst.a[dp + i] = src.a[sp + i];
      else for (let i = 0; i < n; i++) dst.a[dp + i] = src.a[sp + i];
    },
  );
  const setStatic = (name) => (t, v) => {
    vmOf(t).loadClass("java/lang/System").statics[name] = v;
  };
  reg("java/lang/System.setIn0(Ljava/io/InputStream;)V", setStatic("in"));
  reg("java/lang/System.setOut0(Ljava/io/PrintStream;)V", setStatic("out"));
  reg("java/lang/System.setErr0(Ljava/io/PrintStream;)V", setStatic("err"));
  reg("java/lang/System.mapLibraryName(Ljava/lang/String;)Ljava/lang/String;", (t, s) =>
    vmOf(t).str("lib" + vmOf(t).jsstr(s) + ".so"),
  );
  reg("java/lang/System.loadLibrary(Ljava/lang/String;)V", (t, s) => {
    vmOf(t).throwNew(t, "java/lang/UnsatisfiedLinkError", "no " + vmOf(t).jsstr(s) + " in browser");
  });
  reg("java/lang/System.exit(I)V", (t, code) => {
    vmOf(t).log(`[jvm] System.exit(${code})`);
    vmOf(t).exited = true;
    t.state = "terminated";
    return BLOCK;
  });
  reg("java/lang/Runtime.availableProcessors()I", () => 1);
  reg("java/lang/Runtime.freeMemory()J", () => 64n << 20n);
  reg("java/lang/Runtime.totalMemory()J", () => 128n << 20n);
  reg("java/lang/Runtime.maxMemory()J", () => 256n << 20n);
  reg("java/lang/Runtime.gc()V", noop);
  reg("java/lang/Runtime.runFinalization0()V", noop);

  reg("java/lang/Thread.currentThread()Ljava/lang/Thread;", (t) => t.jthread);
  reg("java/lang/Thread.yield()V", (t) => vmOf(t).sleep(t, 0));
  reg("java/lang/Thread.sleep(J)V", (t, ms) => vmOf(t).sleep(t, Number(ms)));
  reg("java/lang/Thread.start0()V", (t, jt) => {
    const vm = vmOf(t);
    const nt = vm.newThread(jt, threadName(vm, jt));
    jt.$t = nt;
    const run = vm.virtualMethod(jt.cls, "run()V");
    vm.pushFrame(nt, run, [jt]);
  });
  reg("java/lang/Thread.isInterrupted(Z)Z", (t, jt, clear) => {
    const th = jt.$t;
    if (!th) return 0;
    const r = th.interrupted ? 1 : 0;
    if (clear) th.interrupted = false;
    return r;
  });
  reg("java/lang/Thread.isAlive()Z", (t, jt) => (jt.$t && jt.$t.state !== "terminated" ? 1 : 0));
  reg("java/lang/Thread.holdsLock(Ljava/lang/Object;)Z", (t, o) =>
    vmOf(t).mon(o).owner === t ? 1 : 0,
  );
  reg("java/lang/Thread.setPriority0(I)V", noop);
  reg("java/lang/Thread.interrupt0()V", (t, jt) => {
    if (jt.$t) vmOf(t).interrupt(jt.$t);
  });
  reg("java/lang/Thread.setNativeName(Ljava/lang/String;)V", (t, jt, n) => {
    if (jt.$t) jt.$t.name = vmOf(t).jsstr(n);
  });
  reg("java/lang/Thread.countStackFrames()I", (t, jt) => (jt.$t ? jt.$t.frames.length : 0));
  reg("java/lang/Thread.stop0(Ljava/lang/Object;)V", noop);
  reg("java/lang/Thread.suspend0()V", noop);
  reg("java/lang/Thread.resume0()V", noop);

  reg("java/lang/Throwable.fillInStackTrace(I)Ljava/lang/Throwable;", (t, ex) => {
    const vm = vmOf(t);
    let tr = vm.trace(t);
    // drop the Throwable constructor frames
    while (tr.length && (tr[0].method === "fillInStackTrace" || tr[0].method === "<init>"))
      tr.shift();
    ex.$trace = tr;
    return ex;
  });
  reg("java/lang/Throwable.getStackTraceDepth()I", (t, ex) => (ex.$trace ? ex.$trace.length : 0));
  reg("java/lang/Throwable.getStackTraceElement(I)Ljava/lang/StackTraceElement;", (t, ex, i) => {
    const vm = vmOf(t);
    const e = ex.$trace[i];
    const c = vm.loadClass("java/lang/StackTraceElement");
    const o = new JObject(c);
    o.declaringClass = vm.str(e.cls);
    o.methodName = vm.str(e.method);
    o.fileName = vm.str(e.file);
    o.lineNumber = e.line;
    return o;
  });

  reg("java/lang/String.intern()Ljava/lang/String;", (t, s) => vmOf(t).intern(vmOf(t).jsstr(s)));
  const buf = new DataView(new ArrayBuffer(8));
  reg("java/lang/Float.floatToRawIntBits(F)I", (t, f) => {
    buf.setFloat32(0, f);
    return buf.getInt32(0);
  });
  reg("java/lang/Float.intBitsToFloat(I)F", (t, i) => {
    buf.setInt32(0, i);
    return buf.getFloat32(0);
  });
  reg("java/lang/Double.doubleToRawLongBits(D)J", (t, d) => {
    buf.setFloat64(0, d);
    return buf.getBigInt64(0);
  });
  reg("java/lang/Double.longBitsToDouble(J)D", (t, l) => {
    buf.setBigInt64(0, l);
    return buf.getFloat64(0);
  });
  for (const fn of [
    "sin",
    "cos",
    "tan",
    "asin",
    "acos",
    "atan",
    "exp",
    "log",
    "log10",
    "sqrt",
    "cbrt",
    "sinh",
    "cosh",
    "tanh",
    "expm1",
    "log1p",
  ]) {
    reg(`java/lang/StrictMath.${fn}(D)D`, (t, x) => Math[fn](x));
  }
  reg("java/lang/StrictMath.atan2(DD)D", (t, a, b) => Math.atan2(a, b));
  reg("java/lang/StrictMath.pow(DD)D", (t, a, b) => Math.pow(a, b));
  reg("java/lang/StrictMath.hypot(DD)D", (t, a, b) => Math.hypot(a, b));
  reg("java/lang/StrictMath.IEEEremainder(DD)D", (t, a, b) => a - b * Math.round(a / b));
  reg("java/util/concurrent/atomic/AtomicLong.VMSupportsCS8()Z", () => 1);

  reg("sun/misc/VM.initialize()V", noop);
  reg("sun/misc/VM.latestUserDefinedLoader()Ljava/lang/ClassLoader;", () => null);
  reg("sun/misc/Signal.findSignal(Ljava/lang/String;)I", () => -1);
  reg("sun/misc/Signal.handle0(IJ)J", () => 0n);

  // Unsafe. Offsets index fieldTable; arrays use the element index directly (base 0, scale 1).
  const U = "sun/misc/Unsafe.";
  reg(U + "arrayBaseOffset(Ljava/lang/Class;)I", () => 0);
  reg(U + "arrayIndexScale(Ljava/lang/Class;)I", () => 1);
  reg(U + "addressSize()I", () => 8);
  reg(U + "pageSize()I", () => 4096);
  const fieldSlot = (t, fo) => fo.slot;
  reg(U + "objectFieldOffset(Ljava/lang/reflect/Field;)J", (t, u, fo) => BigInt(fieldSlot(t, fo)));
  reg(U + "staticFieldOffset(Ljava/lang/reflect/Field;)J", (t, u, fo) => BigInt(fieldSlot(t, fo)));
  reg(U + "staticFieldBase(Ljava/lang/reflect/Field;)Ljava/lang/Object;", (t, u, fo) => fo.clazz);
  const uget = (o, off) => {
    if (o instanceof JArray) return o.a[Number(off)];
    const e = fieldTable[Number(off)];
    return e.static ? e.cls.statics[e.key] : o[e.key];
  };
  const uset = (o, off, v) => {
    if (o instanceof JArray) {
      o.a[Number(off)] = v;
      return;
    }
    const e = fieldTable[Number(off)];
    if (e.static) e.cls.statics[e.key] = v;
    else o[e.key] = v;
  };
  for (const [T, d] of [
    ["Int", "I"],
    ["Long", "J"],
    ["Object", "Ljava/lang/Object;"],
    ["Boolean", "Z"],
    ["Byte", "B"],
    ["Short", "S"],
    ["Char", "C"],
    ["Float", "F"],
    ["Double", "D"],
  ]) {
    for (const vol of ["", "Volatile"]) {
      reg(`${U}get${T}${vol}(Ljava/lang/Object;J)${d}`, (t, u, o, off) => uget(o, off));
      reg(`${U}put${T}${vol}(Ljava/lang/Object;J${d})V`, (t, u, o, off, v) => uset(o, off, v));
    }
    reg(`${U}putOrdered${T}(Ljava/lang/Object;J${d})V`, (t, u, o, off, v) => uset(o, off, v));
  }
  for (const [T, d] of [
    ["Int", "I"],
    ["Long", "J"],
    ["Object", "Ljava/lang/Object;"],
  ]) {
    reg(`${U}compareAndSwap${T}(Ljava/lang/Object;J${d}${d})Z`, (t, u, o, off, exp, v) => {
      if (uget(o, off) !== exp) return 0;
      uset(o, off, v);
      return 1;
    });
  }
  reg(U + "ensureClassInitialized(Ljava/lang/Class;)V", (t, u, c) => {
    vmOf(t).ensureInitSync(t, C(c));
  });
  reg(U + "shouldBeInitialized(Ljava/lang/Class;)Z", (t, u, c) =>
    C(c).state !== "initialized" ? 1 : 0,
  );
  reg(U + "allocateInstance(Ljava/lang/Class;)Ljava/lang/Object;", (t, u, c) => {
    vmOf(t).ensureInitSync(t, C(c));
    return new JObject(C(c));
  });
  reg(U + "throwException(Ljava/lang/Throwable;)V", (t, u, e) => {
    throw new JavaThrow(e);
  });
  reg(U + "park(ZJ)V", (t, u, abs, time) => {
    const vm = vmOf(t);
    if (t.unparked) {
      t.unparked = false;
      return undefined;
    }
    let ms = Infinity;
    if (abs) ms = Number(time) - Date.now();
    else if (time > 0n) ms = Number(time) / 1e6;
    t.state = "sleeping";
    t.wakeAt = vm.now() + ms;
    t.parked = true;
    t.resume = () => {
      t.parked = false;
      t.unparked = false;
      return undefined;
    };
    return BLOCK;
  });
  reg(U + "unpark(Ljava/lang/Object;)V", (t, u, jt) => {
    const th = jt && jt.$t;
    if (!th) return;
    if (th.parked) th.wakeAt = 0;
    else th.unparked = true;
  });
  // raw memory, for java.nio.Bits and friends
  const heap = { buf: new DataView(new ArrayBuffer(1 << 16)), top: 8 };
  reg(U + "allocateMemory(J)J", (t, u, n) => {
    const p = heap.top;
    heap.top += (Number(n) + 7) & ~7;
    if (heap.top > heap.buf.byteLength) {
      const nb = new Uint8Array(Math.max(heap.top * 2, heap.buf.byteLength * 2));
      nb.set(new Uint8Array(heap.buf.buffer));
      heap.buf = new DataView(nb.buffer);
    }
    return BigInt(p);
  });
  reg(U + "freeMemory(J)V", noop);
  reg(U + "putLong(JJ)V", (t, u, a, v) => heap.buf.setBigInt64(Number(a), v, true));
  reg(U + "getLong(J)J", (t, u, a) => heap.buf.getBigInt64(Number(a), true));
  reg(U + "getByte(J)B", (t, u, a) => heap.buf.getInt8(Number(a)));
  reg(U + "putByte(JB)V", (t, u, a, v) => heap.buf.setInt8(Number(a), v));

  const runAction = (t, action) => {
    const vm = vmOf(t);
    if (action === null) vm.throwNew(t, "java/lang/NullPointerException", null);
    return vm.callFromNative(t, vm.virtualMethod(action.cls, "run()Ljava/lang/Object;"), [action]);
  };
  for (const iface of ["PrivilegedAction", "PrivilegedExceptionAction"]) {
    reg(
      `java/security/AccessController.doPrivileged(Ljava/security/${iface};)Ljava/lang/Object;`,
      (t, a) => runAction(t, a),
    );
    reg(
      `java/security/AccessController.doPrivileged(Ljava/security/${iface};Ljava/security/AccessControlContext;)Ljava/lang/Object;`,
      (t, a) => runAction(t, a),
    );
  }
  reg(
    "java/security/AccessController.getStackAccessControlContext()Ljava/security/AccessControlContext;",
    () => null,
  );
  reg(
    "java/security/AccessController.getInheritedAccessControlContext()Ljava/security/AccessControlContext;",
    () => null,
  );

  // Everything is on the boot class path.
  reg(
    "java/lang/ClassLoader.findBootstrapClass(Ljava/lang/String;)Ljava/lang/Class;",
    (t, cl, n) => {
      const vm = vmOf(t);
      const c = vm.loadClass(vm.jsstr(n).replace(/\./g, "/"));
      return c ? vm.classObject(c) : null;
    },
  );
  reg("java/lang/ClassLoader.findLoadedClass0(Ljava/lang/String;)Ljava/lang/Class;", (t, cl, n) => {
    const vm = vmOf(t);
    const c = vm.classes.get(vm.jsstr(n).replace(/\./g, "/"));
    return c ? vm.classObject(c) : null;
  });
  let systemLoader = null;
  reg("java/lang/ClassLoader.getSystemClassLoader()Ljava/lang/ClassLoader;", (t) => {
    if (!systemLoader) {
      const vm = vmOf(t);
      const c = vm.loadClass("websim/SystemLoader");
      vm.ensureInitSync(t, c);
      systemLoader = new JObject(c);
      vm.invokeSync(t, c.methods.get("<init>()V"), [systemLoader]);
    }
    return systemLoader;
  });
  // Resources come straight from the jars.
  const resource = (t, n) => {
    const vm = vmOf(t);
    let name = vm.jsstr(n);
    if (name.startsWith("/")) name = name.slice(1);
    const bytes = vm.findBytes(name);
    if (!bytes) return null;
    return newByteStream(t, bytes);
  };
  const newByteStream = (t, bytes) => {
    const vm = vmOf(t);
    const c = vm.loadClass("java/io/ByteArrayInputStream");
    vm.ensureInitSync(t, c);
    const o = new JObject(c);
    const arr = vm.newArray("[B", bytes.length);
    arr.a.set(new Int8Array(bytes.buffer, bytes.byteOffset, bytes.length));
    vm.invokeSync(t, c.methods.get("<init>([B)V"), [o, arr]);
    return o;
  };
  reg(
    "java/lang/ClassLoader.getSystemResourceAsStream(Ljava/lang/String;)Ljava/io/InputStream;",
    resource,
  );
  reg(
    "java/lang/Class.getResourceAsStream(Ljava/lang/String;)Ljava/io/InputStream;",
    (t, cls, n) => {
      const vm = vmOf(t);
      let name = vm.jsstr(n);
      if (!name.startsWith("/")) {
        const c = C(cls).name;
        const i = c.lastIndexOf("/");
        if (i >= 0) name = c.slice(0, i + 1) + name;
      }
      return resource(t, vm.str(name));
    },
  );
  reg(
    "java/lang/ClassLoader.getResourceAsStream(Ljava/lang/String;)Ljava/io/InputStream;",
    (t, cl, n) => resource(t, n),
  );
  // No service providers or URL resources: ServiceLoader and friends see empty enumerations.
  const emptyEnum = (t) => {
    const vm = vmOf(t);
    const c = vm.loadClass("java/util/Collections");
    vm.ensureInitSync(t, c);
    return vm.invokeSync(t, c.methods.get("emptyEnumeration()Ljava/util/Enumeration;"), []);
  };
  reg("java/lang/ClassLoader.getSystemResources(Ljava/lang/String;)Ljava/util/Enumeration;", (t) =>
    emptyEnum(t),
  );
  reg(
    "java/lang/ClassLoader.getBootstrapResources(Ljava/lang/String;)Ljava/util/Enumeration;",
    (t) => emptyEnum(t),
  );
  reg("java/lang/ClassLoader.getResources(Ljava/lang/String;)Ljava/util/Enumeration;", (t) =>
    emptyEnum(t),
  );
  reg("java/lang/ClassLoader.getSystemResource(Ljava/lang/String;)Ljava/net/URL;", () => null);
  reg("java/lang/ClassLoader.getResource(Ljava/lang/String;)Ljava/net/URL;", () => null);
  reg("java/lang/Class.getResource(Ljava/lang/String;)Ljava/net/URL;", () => null);

  for (const c of [
    "java/io/FileDescriptor",
    "java/io/FileInputStream",
    "java/io/FileOutputStream",
    "java/io/UnixFileSystem",
    "java/io/RandomAccessFile",
    "java/io/ObjectStreamClass",
  ])
    reg(c + ".initIDs()V", noop);
  reg("java/io/FileDescriptor.set(I)J", () => 0n);
  reg("java/io/FileDescriptor.sync()V", noop);
  const fdOf = (stream) => stream.fd.fd;
  const decoder = new TextDecoder("utf-8");
  reg("java/io/FileOutputStream.writeBytes([BIIZ)V", (t, s, b, off, len) => {
    const fd = fdOf(s);
    const bytes = new Uint8Array(b.a.buffer, b.a.byteOffset + off, len);
    if (fd === 1) stdout(decoder.decode(bytes));
    else if (fd === 2) stderr(decoder.decode(bytes));
    else fileWrite(t, fd, bytes);
  });
  reg("java/io/FileOutputStream.write(IZ)V", (t, s, v) => {
    const fd = fdOf(s);
    const bytes = new Uint8Array([v & 0xff]);
    if (fd === 1) stdout(String.fromCharCode(v & 0xff));
    else if (fd === 2) stderr(String.fromCharCode(v & 0xff));
    else fileWrite(t, fd, bytes);
  });
  const fileWrite = (t, fd, bytes) => {
    const f = openFiles.get(fd);
    if (!f) vmOf(t).throwNew(t, "java/io/IOException", "Stream Closed");
    const need = f.pos + bytes.length;
    if (need > f.data.length) {
      const nd = new Uint8Array(Math.max(need, f.data.length * 2));
      nd.set(f.data.subarray(0, f.size));
      f.data = nd;
    }
    f.data.set(bytes, f.pos);
    f.pos += bytes.length;
    f.size = Math.max(f.size, f.pos);
    f.dirty = true;
  };
  const open = (t, s, path, mode) => {
    const vm = vmOf(t);
    const p = vfs.norm(vm.jsstr(path));
    if (mode === "r" && !vfs.files.has(p) && vfs.remote.has(p)) {
      const r = vfs.remote.get(p);
      const fd = nextFd++;
      openFiles.set(fd, { path: p, remote: r, size: r.size, pos: 0, write: false });
      s.fd.fd = fd;
      return;
    }
    let data = vfs.read(p);
    if (mode === "r" && !data)
      vm.throwNew(t, "java/io/FileNotFoundException", p + " (No such file or directory)");
    if (mode !== "r" && !vfs.dirs.has(vfs.parent(p)))
      vm.throwNew(t, "java/io/FileNotFoundException", p + " (No such file or directory)");
    if (mode === "w" || !data) data = new Uint8Array(0);
    const fd = nextFd++;
    openFiles.set(fd, {
      path: p,
      data: data.slice(),
      size: data.length,
      pos: mode === "a" ? data.length : 0,
      write: mode !== "r",
      dirty: mode === "w",
    });
    if (mode !== "r") vfs.write(p, data);
    s.fd.fd = fd;
  };
  reg("java/io/FileInputStream.open0(Ljava/lang/String;)V", (t, s, p) => open(t, s, p, "r"));
  reg("java/io/FileOutputStream.open0(Ljava/lang/String;Z)V", (t, s, p, append) =>
    open(t, s, p, append ? "a" : "w"),
  );
  const closeFd = (fd) => {
    const f = openFiles.get(fd);
    if (!f) return;
    if (f.write && f.dirty) vfs.write(f.path, f.data.slice(0, f.size));
    openFiles.delete(fd);
  };
  for (const c of [
    "java/io/FileInputStream",
    "java/io/FileOutputStream",
    "java/io/RandomAccessFile",
  ]) {
    reg(c + ".close0()V", (t, s) => {
      closeFd(fdOf(s));
      s.fd.fd = -1;
    });
  }
  // Read up to len bytes at the file position into a Java byte[]; -1 at EOF.
  const readInto = (f, b, off, len) => {
    if (f.pos >= f.size) return -1;
    const n = Math.min(len, f.size - f.pos);
    const src = f.remote ? f.remote.read(f.pos, n) : f.data.subarray(f.pos, f.pos + n);
    b.a.set(new Int8Array(src.buffer, src.byteOffset, n), off);
    f.pos += n;
    return n;
  };
  const readByte = (f) => {
    if (f.pos >= f.size) return -1;
    const v = f.remote ? f.remote.read(f.pos, 1)[0] : f.data[f.pos];
    f.pos++;
    return v;
  };
  reg("java/io/FileInputStream.readBytes([BII)I", (t, s, b, off, len) => {
    const fd = fdOf(s);
    if (fd === 0) return -1;
    const f = openFiles.get(fd);
    if (!f) vmOf(t).throwNew(t, "java/io/IOException", "Stream Closed");
    return readInto(f, b, off, len);
  });
  reg("java/io/FileInputStream.read0()I", (t, s) => {
    const f = openFiles.get(fdOf(s));
    return f ? readByte(f) : -1;
  });
  reg("java/io/FileInputStream.available0()I", (t, s) => {
    const f = openFiles.get(fdOf(s));
    return f ? f.size - f.pos : 0;
  });
  reg("java/io/FileInputStream.skip0(J)J", (t, s, n) => {
    const f = openFiles.get(fdOf(s));
    const k = Math.min(Number(n), f.size - f.pos);
    f.pos += k;
    return BigInt(k);
  });
  reg("java/io/RandomAccessFile.open0(Ljava/lang/String;I)V", (t, s, p, mode) =>
    open(t, s, p, mode & 2 ? "rw" : "r"),
  );
  reg("java/io/RandomAccessFile.read0()I", (t, s) => readByte(openFiles.get(fdOf(s))));
  reg("java/io/RandomAccessFile.readBytes([BII)I", (t, s, b, off, len) =>
    readInto(openFiles.get(fdOf(s)), b, off, len),
  );
  reg("java/io/RandomAccessFile.write0(I)V", (t, s, v) =>
    fileWrite(t, fdOf(s), new Uint8Array([v & 0xff])),
  );
  reg("java/io/RandomAccessFile.writeBytes([BII)V", (t, s, b, off, len) =>
    fileWrite(t, fdOf(s), new Uint8Array(b.a.buffer, b.a.byteOffset + off, len)),
  );
  reg("java/io/RandomAccessFile.getFilePointer()J", (t, s) => BigInt(openFiles.get(fdOf(s)).pos));
  reg("java/io/RandomAccessFile.seek0(J)V", (t, s, p) => {
    openFiles.get(fdOf(s)).pos = Number(p);
  });
  reg("java/io/RandomAccessFile.length()J", (t, s) => BigInt(openFiles.get(fdOf(s)).size));
  reg("java/io/RandomAccessFile.setLength(J)V", (t, s, n) => {
    const f = openFiles.get(fdOf(s));
    n = Number(n);
    if (n > f.data.length) {
      const nd = new Uint8Array(n);
      nd.set(f.data.subarray(0, f.size));
      f.data = nd;
    }
    f.size = n;
    f.pos = Math.min(f.pos, n);
    f.dirty = true;
  });

  // UnixFileSystem over the VFS
  const FS = "java/io/UnixFileSystem.";
  const pathOf = (t, file) => vfs.norm(vmOf(t).jsstr(file.path));
  reg(FS + "canonicalize0(Ljava/lang/String;)Ljava/lang/String;", (t, fs, p) =>
    vmOf(t).str(vfs.norm(vmOf(t).jsstr(p))),
  );
  reg(FS + "getBooleanAttributes0(Ljava/io/File;)I", (t, fs, f) => {
    const p = pathOf(t, f);
    if (vfs.dirs.has(p)) return 0x01 | 0x04; // exists | directory
    if (vfs.exists(p)) return 0x01 | 0x02; // exists | regular
    return 0;
  });
  reg(FS + "checkAccess(Ljava/io/File;I)Z", (t, fs, f) => {
    const p = pathOf(t, f);
    return vfs.dirs.has(p) || vfs.exists(p) ? 1 : 0;
  });
  reg(FS + "getLastModifiedTime(Ljava/io/File;)J", (t, fs, f) =>
    BigInt(vfs.mtime.get(pathOf(t, f)) || 0),
  );
  reg(FS + "getLength(Ljava/io/File;)J", (t, fs, f) => {
    return BigInt(vfs.size(pathOf(t, f)));
  });
  reg(FS + "setPermission(Ljava/io/File;IZZ)Z", () => 1);
  reg(FS + "setLastModifiedTime(Ljava/io/File;J)Z", (t, fs, f, ms) => {
    vfs.mtime.set(pathOf(t, f), Number(ms));
    return 1;
  });
  reg(FS + "setReadOnly(Ljava/io/File;)Z", () => 1);
  reg(FS + "createFileExclusively(Ljava/lang/String;)Z", (t, fs, p) => {
    const path = vfs.norm(vmOf(t).jsstr(p));
    if (vfs.files.has(path) || vfs.dirs.has(path)) return 0;
    if (!vfs.dirs.has(vfs.parent(path)))
      vmOf(t).throwNew(t, "java/io/IOException", "No such file or directory");
    vfs.write(path, new Uint8Array(0));
    return 1;
  });
  reg(FS + "delete0(Ljava/io/File;)Z", (t, fs, f) => {
    const p = pathOf(t, f);
    if (vfs.remote.delete(p)) return 1;
    if (vfs.files.delete(p)) {
      if (vfs.onChange) vfs.onChange(p);
      return 1;
    }
    if (vfs.dirs.has(p) && vfs.list(p).length === 0) {
      vfs.dirs.delete(p);
      return 1;
    }
    return 0;
  });
  reg(FS + "list(Ljava/io/File;)[Ljava/lang/String;", (t, fs, f) => {
    const vm = vmOf(t);
    const p = pathOf(t, f);
    if (!vfs.dirs.has(p)) return null;
    const names = vfs.list(p);
    const arr = vm.newArray("[Ljava/lang/String;", names.length);
    names.forEach((n, i) => (arr.a[i] = vm.str(n)));
    return arr;
  });
  reg(FS + "createDirectory(Ljava/io/File;)Z", (t, fs, f) => {
    const p = pathOf(t, f);
    if (vfs.dirs.has(p) || vfs.files.has(p) || !vfs.dirs.has(vfs.parent(p))) return 0;
    vfs.dirs.add(p);
    return 1;
  });
  reg(FS + "rename0(Ljava/io/File;Ljava/io/File;)Z", (t, fs, a, b) => {
    const pa = pathOf(t, a),
      pb = pathOf(t, b);
    const d = vfs.read(pa);
    if (!d) return 0;
    vfs.files.delete(pa);
    vfs.write(pb, d);
    return 1;
  });
  reg(FS + "getSpace(Ljava/io/File;I)J", () => 256n << 20n);

  const crcTable = new Int32Array(256);
  for (let n = 0; n < 256; n++) {
    let c = n;
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    crcTable[n] = c;
  }
  const crc = (c, bytes, off, len) => {
    c = ~c;
    for (let i = off; i < off + len; i++) c = crcTable[(c ^ bytes[i]) & 255] ^ (c >>> 8);
    return ~c;
  };
  reg("java/util/zip/CRC32.update(II)I", (t, c, b) => crc(c, [b], 0, 1));
  reg("java/util/zip/CRC32.updateBytes(I[BII)I", (t, c, b, off, len) => crc(c, b.a, off, len));

  // java.util.zip.Inflater: whole-stream decode once enough input has arrived.
  const zstreams = new Map();
  let nextZ = 1;
  reg("java/util/zip/Inflater.initIDs()V", noop);
  reg("java/util/zip/Inflater.init(Z)J", (t, nowrap) => {
    const id = nextZ++;
    zstreams.set(id, { nowrap: !!nowrap, input: new Uint8Array(0), out: null, outPos: 0 });
    return BigInt(id);
  });
  reg("java/util/zip/Inflater.reset(J)V", (t, id) => {
    const z = zstreams.get(Number(id));
    z.input = new Uint8Array(0);
    z.out = null;
    z.outPos = 0;
  });
  reg("java/util/zip/Inflater.end(J)V", (t, id) => {
    zstreams.delete(Number(id));
  });
  reg("java/util/zip/Inflater.getAdler(J)I", () => 1);
  reg("java/util/zip/Inflater.setDictionary(J[BII)V", noop);
  reg("java/util/zip/Inflater.inflateBytes(J[BII)I", (t, inf, id, b, off, len) => {
    const z = zstreams.get(Number(id));
    const feed = inf.len;
    if (feed > 0) {
      const src = new Uint8Array(inf.buf.a.buffer, inf.buf.a.byteOffset + inf.off, feed);
      const all = new Uint8Array(z.input.length + feed);
      all.set(z.input);
      all.set(src, z.input.length);
      z.input = all;
      inf.off += feed;
      inf.len = 0;
    }
    if (!z.out) {
      const head = z.nowrap ? 0 : 2;
      if (z.input.length <= head) return 0;
      let r;
      try {
        r = inflate(z.input.subarray(head));
      } catch (e) {
        if (e instanceof Incomplete) return 0;
        vmOf(t).throwNew(t, "java/util/zip/DataFormatException", e.message);
      }
      z.out = r.out;
      const leftover = z.input.length - (head + r.used + (z.nowrap ? 0 : 4));
      if (leftover > 0) {
        const back = Math.min(leftover, feed);
        inf.off -= back;
        inf.len = back;
      }
    }
    const n = Math.min(len, z.out.length - z.outPos);
    b.a.set(new Int8Array(z.out.buffer, z.out.byteOffset + z.outPos, n), off);
    z.outPos += n;
    if (z.outPos >= z.out.length) inf.finished = 1;
    return n;
  });

  reg("java/util/TimeZone.getSystemTimeZoneID(Ljava/lang/String;)Ljava/lang/String;", (t) =>
    vmOf(t).str(Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC"),
  );
  reg("java/util/TimeZone.getSystemGMTOffsetID()Ljava/lang/String;", (t) => vmOf(t).str("GMT"));
  reg("java/lang/Package.getSystemPackage0(Ljava/lang/String;)Ljava/lang/String;", () => null);
  reg("java/lang/ref/Finalizer.invokeFinalizeMethod(Ljava/lang/Object;)V", noop);

  return { natives: N, vfs, fieldTable };
}

export function threadName(vm, jt) {
  const n = jt.name;
  if (!n) return "thread";
  if (n instanceof JArray) return String.fromCharCode(...n.a);
  return vm.jsstr(n);
}
