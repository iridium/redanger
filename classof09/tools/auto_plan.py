"""Auto Play's route through the game, worked out ahead of time from the bytecode.

The plan is a list of segments. Each starts at a scene label (fast-forwarding
to the menu it was left at, except the first), takes the listed menu choices
in order as it plays, and ends at a stop point, or at an ending (RETURN).

How the route is chosen:
- Identical menus (the script copies some, e.g. the class choice after each
  intro route) count as one menu.
- A "flavor" menu is one where every option can still reach the same
  endings. Its options play back to back, each only as far as the first point
  where they all rejoin; the last option carries on from there.
- Any other menu is a branch: every option is played to its end.
- A path stops as soon as it reaches a scene that has already played, or a
  menu that has already been explored, since everything after that has been
  (or is being) covered elsewhere.
"""
import struct

OP_SIZE = {1: 5, 2: 7, 3: 3, 4: 5, 5: 3, 6: 2, 7: 11, 9: 4, 10: 1, 11: 4, 13: 0, 14: 4}
OP_PLAY, OP_STOP, OP_MENU, OP_JUMP, OP_ENDING, OP_RETURN, OP_LABEL = 4, 5, 8, 9, 10, 12, 13
NONE16, NONE32 = 0xFFFF, 0xFFFFFFFF
EXIT = ('X', 0)


def u16(b, p): return struct.unpack_from('>H', b, p)[0]
def s32(b, p): return struct.unpack_from('>i', b, p)[0]


def options(code, pc):
    n = code[pc + 1]
    return [(u16(code, pc + 2 + 6 * i), s32(code, pc + 4 + 6 * i)) for i in range(n)]


def next_point(code, pc):
    """From pc, the first LABEL or MENU reached (('L'|'M', pc)), or EXIT; and the endings passed."""
    ends = []
    while True:
        op = code[pc]
        if op == OP_LABEL:
            return ('L', pc), ends
        if op == OP_MENU:
            return ('M', pc), ends
        if op == OP_JUMP:
            pc = s32(code, pc + 1)
            continue
        if op == OP_ENDING:
            ends.append(code[pc + 1])
        if op == OP_RETURN or op not in OP_SIZE:
            return EXIT, ends
        pc += 1 + OP_SIZE[op]


class Graph:
    def __init__(self, code, start_pc):
        self.code = code
        self.succ, self.ends = {}, {}
        todo = [next_point(code, start_pc)[0]]
        while todo:
            n = todo.pop()
            if n in self.succ or n == EXIT:
                continue
            if n[0] == 'L':
                p, e = next_point(code, n[1] + 1)
                self.succ[n], self.ends[n] = [p], e
            else:
                self.succ[n], self.ends[n] = [], []
                for _, target in options(code, n[1]):
                    p, e = next_point(code, target)
                    self.succ[n].append(p)
            todo += self.succ[n]
        self._canon()
        self._reach()
        self._postdom()

    def _canon(self):
        """Menus with the same options leading to the same places are one menu."""
        menus = sorted(n for n in self.succ if n[0] == 'M')
        self.canon = {m: m for m in menus}
        while True:
            first, changed = {}, False
            for m in menus:
                sig = tuple((t, self.canon.get(p, p)) for (t, _), p in zip(options(self.code, m[1]), self.succ[m]))
                first.setdefault(sig, m)
                if self.canon[m] != first[sig]:
                    self.canon[m], changed = first[sig], True
            if not changed:
                return

    def _reach(self):
        memo = {EXIT: frozenset()}
        def reach(n):
            if n not in memo:
                memo[n] = frozenset()   # (DAG; guards a malformed loop)
                memo[n] = frozenset(self.ends[n]).union(*(reach(s) for s in self.succ[n]))
            return memo[n]
        self.reach = reach

    def _postdom(self):
        """Post-dominators over the point graph (menus merged), for rejoin points."""
        nodes = list(self.succ) + [EXIT]
        allset = frozenset(self.key(n) for n in nodes)
        pd = {self.key(n): allset for n in nodes}
        pd[EXIT] = frozenset([EXIT])
        changed = True
        while changed:
            changed = False
            for n in self.succ:
                k = self.key(n)
                new = frozenset([k]).union(frozenset.intersection(*(pd[self.key(s)] for s in self.succ[n])))
                if new != pd[k]:
                    pd[k], changed = new, True
        self.pd = pd

    def key(self, n):
        return self.canon.get(n, n)

    def flavor(self, m):
        return len({self.reach(s) for s in self.succ[m]}) == 1

    def rejoin(self, m):
        """The nearest point every option of menu m passes through (EXIT if none)."""
        k = self.key(m)
        cand = self.pd[k] - {k}
        return max(cand, key=lambda p: len(self.pd[p]))


def plan(code, start_pc):
    """[(start pc, ambient or -1, fast-forward?, [choices], stop pc or -1)]"""
    g = Graph(code, start_pc)
    segs, played, explored = [], set(), set()

    def walk(pc, amb, stops, label):
        seg = segs[-1]
        while True:
            op = code[pc]
            if op == OP_LABEL:
                if ('L', pc) in stops or pc in played:
                    seg[4] = pc
                    return
                played.add(pc)
                label = (pc, amb)
            elif op == OP_MENU:
                k = g.key(('M', pc))
                if k in stops or k in explored:
                    seg[4] = pc
                    return
                explored.add(k)
                explore(pc, amb, stops, label)
                return
            elif op == OP_PLAY and code[pc + 1] == 0:
                amb = u16(code, pc + 2)
            elif op == OP_STOP and code[pc + 1] == 0:
                amb = -1
            elif op == OP_JUMP:
                pc = s32(code, pc + 1)
                continue
            elif op == OP_RETURN or op not in OP_SIZE:
                return
            pc += 1 + OP_SIZE[op]

    def explore(pc, amb, stops, label):
        m = ('M', pc)
        opts = options(code, pc)
        rejoin = g.rejoin(m) if g.flavor(m) else EXIT
        for i, (_, target) in enumerate(opts):
            if i:
                segs.append([label[0], label[1], True, [], -1])
            segs[-1][3].append(i)
            last = i == len(opts) - 1
            walk(target, amb, stops | {rejoin} if rejoin != EXIT and not last else stops, label)

    segs.append([start_pc, -1, False, [], -1])
    walk(start_pc, -1, frozenset(), (start_pc, -1))
    return segs


def encode(segs):
    out = bytearray(struct.pack('>H', len(segs)))
    for start, amb, fast, choices, stop in segs:
        out += struct.pack('>IHBIB', start, amb & NONE16, int(fast), stop & NONE32, len(choices)) + bytes(choices)
    return out


def decode(b, p):
    """Reads an encoded plan at b[p:]: the segment list."""
    n = u16(b, p)
    p += 2
    segs = []
    for _ in range(n):
        start, amb, fast, stop, nc = struct.unpack_from('>IHBIB', b, p)
        p += 12
        segs.append((start, -1 if amb == NONE16 else amb, bool(fast), list(b[p:p + nc]),
                     -1 if stop == NONE32 else stop))
        p += nc
    return segs
