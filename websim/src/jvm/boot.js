// Brings up a JVM: main thread + thread groups, System properties and streams, then main().

import { JVM, JObject } from "./jvm.js";
import { makeNatives } from "./natives.js";

export function createVM({ jars, props = {}, natives: extra = {}, vfs, stdout, stderr, log }) {
  const base = makeNatives({ vfs, stdout, stderr });
  const vm = new JVM({ jars, natives: { ...base.natives, ...extra }, log });
  vm.vfs = base.vfs;
  vm.fieldTable = base.fieldTable;
  vm.props = props;
  return vm;
}

const method = (vm, cls, nd) => {
  const c = vm.loadClass(cls);
  const m = vm.findMethod(c, nd);
  if (!m) throw new Error(`no ${cls}.${nd}`);
  return m;
};

function construct(vm, t, cls, desc, args) {
  const c = vm.loadClass(cls);
  vm.ensureInitSync(t, c);
  const o = new JObject(c);
  vm.invokeSync(t, c.methods.get("<init>" + desc), [o, ...args]);
  return o;
}

export function bootVM(vm) {
  // Main thread: allocate the java.lang.Thread without running its constructor
  // (that needs a current thread to inherit from).
  const TC = vm.loadClass("java/lang/Thread");
  const jt = new JObject(TC);
  const t = vm.newThread(jt, "main");
  jt.$t = t;
  vm.current = t;
  for (const c of [
    "java/lang/Object",
    "java/lang/String",
    "java/lang/System",
    "java/lang/Class",
    "java/lang/ThreadGroup",
    "java/lang/Thread",
  ])
    vm.ensureInitSync(t, vm.loadClass(c));

  const system = construct(vm, t, "java/lang/ThreadGroup", "()V", []);
  const main = construct(
    vm,
    t,
    "java/lang/ThreadGroup",
    "(Ljava/lang/ThreadGroup;Ljava/lang/String;)V",
    [system, vm.str("main")],
  );
  jt.group = main;
  jt.priority = 5;
  jt.threadStatus = 5;
  jt.name =
    typeof TC.fields.get("name").desc === "string" && TC.fields.get("name").desc === "[C"
      ? vm.newArray("[C", 0)
      : vm.str("main");
  if (TC.fields.get("name").desc === "[C") {
    const a = vm.newArray("[C", 4);
    a.a.set([109, 97, 105, 110]);
    jt.name = a;
  }
  jt.tid = 1n;
  vm.invokeSync(t, method(vm, "java/lang/ThreadGroup", "add(Ljava/lang/Thread;)V"), [main, jt]);

  const props = construct(vm, t, "java/util/Properties", "()V", []);
  const all = {
    "java.version": "1.8.0",
    "java.vendor": "Re:Danger",
    "java.home": "/jre",
    "java.class.path": "",
    "java.vm.name": "websim",
    "java.specification.version": "1.8",
    "os.name": "Linux",
    "os.arch": "wasm",
    "os.version": "1",
    "file.separator": "/",
    "path.separator": ":",
    "line.separator": "\n",
    "user.name": "danger",
    "user.home": "/home",
    "user.dir": "/",
    "java.io.tmpdir": "/tmp",
    "file.encoding": "UTF-8",
    "sun.jnu.encoding": "UTF-8",
    "user.language": "en",
    "user.country": "US",
    "sun.reflect.inflationThreshold": "2147483647",
    "sun.reflect.noInflation": "false",
    "java.awt.headless": "true",
    ...vm.props,
  };
  const setProp = method(
    vm,
    "java/util/Properties",
    "setProperty(Ljava/lang/String;Ljava/lang/String;)Ljava/lang/Object;",
  );
  for (const [k, v] of Object.entries(all))
    vm.invokeSync(t, setProp, [props, vm.str(k), vm.str(String(v))]);
  vm.invokeSync(t, method(vm, "sun/misc/VM", "saveAndRemoveProperties(Ljava/util/Properties;)V"), [
    props,
  ]);
  const S = vm.loadClass("java/lang/System");
  S.statics.props = props;
  S.statics.lineSeparator = vm.str("\n");

  const FD = vm.loadClass("java/io/FileDescriptor");
  vm.ensureInitSync(t, FD);
  const ps = (fd) => {
    const fos = construct(vm, t, "java/io/FileOutputStream", "(Ljava/io/FileDescriptor;)V", [fd]);
    return construct(vm, t, "java/io/PrintStream", "(Ljava/io/OutputStream;Z)V", [fos, 1]);
  };
  S.statics.out = ps(FD.statics.out);
  S.statics.err = ps(FD.statics.err);
  S.statics.in = construct(vm, t, "java/io/FileInputStream", "(Ljava/io/FileDescriptor;)V", [
    FD.statics.in,
  ]);
  vm.invokeSync(t, method(vm, "java/lang/System", "setJavaLangAccess()V"), []);
  vm.invokeSync(t, method(vm, "sun/misc/VM", "booted()V"), []);
  vm.current = null;
  return t;
}

// Start `cls.main(args)` on the main thread.
export function runMain(vm, t, cls, args = []) {
  const c = vm.loadClass(cls.replace(/\./g, "/"));
  if (!c) throw new Error(`main class ${cls} not found`);
  vm.ensureInitSync(t, c);
  const arr = vm.newArray("[Ljava/lang/String;", args.length);
  args.forEach((a, i) => (arr.a[i] = vm.str(a)));
  vm.pushFrame(t, c.methods.get("main([Ljava/lang/String;)V"), [arr]);
}
