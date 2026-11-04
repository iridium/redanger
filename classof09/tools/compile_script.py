#!/usr/bin/env python3
"""Compile the decompiled Class of '09 script.rpy into the C9 script format.

Handles exactly the Ren'Py subset the game uses: scene/show/hide with ATL
(linear, pause, parallel, align/alpha/zoom/xzoom), `with Pause(t)`, ambient
play/stop with fades, setVoiceTrack/setWait/speak voice-synced dialogue,
menus, jumps, ending bookkeeping and movies.

Usage: compile_script.py script.rpy images_dir out_dir
Writes out_dir/script.c9s and out_dir/script.json (debug dump), and
out_dir/manifest.json (images/sounds/movies the asset encoders must produce).
See ../FORMATS.md for the binary layout.
"""
import ast, json, os, re, struct, sys

import auto_plan

class Node:
    def __init__(self, text, line):
        self.text, self.line, self.kids = text, line, []
    def __repr__(self):
        return f"Node({self.text!r}, {len(self.kids)} kids)"

def parse_blocks(lines):
    root = Node('<root>', 0)
    stack = [(-1, root)]
    for no, raw in enumerate(lines, 1):
        s = raw.rstrip()
        if not s.strip() or s.strip().startswith('#'):
            continue
        ind = len(s) - len(s.lstrip())
        while stack[-1][0] >= ind:
            stack.pop()
        n = Node(s.strip(), no)
        stack[-1][1].kids.append(n)
        stack.append((ind, n))
    return root

# The ending tracker's first-time tip (text is baked into gui/ending_tracker/confirm_bubble.png).
TRACKER_TIP = ("Thought you were done after one ending? Not by a longshot... Explore every fate of "
               "Nicole and you just might see things her way... Or you're some loser who gets a "
               "trigger response when talking to women.")

class Table:
    def __init__(self):
        self.items, self.idx = [], {}
    def get(self, k):
        if k not in self.idx:
            self.idx[k] = len(self.items)
            self.items.append(k)
        return self.idx[k]

POSITIONS = {
    'leftstage': {'xalign': 0.0}, 'leftcenterstage': {'xalign': 0.33},
    'centerstage': {'xalign': 0.5}, 'rightcenterstage': {'xalign': 0.66},
    'rightstage': {'xalign': 1.0}, 'off_right': {'xalign': 1.4},
    'off_left': {'xalign': -0.4}, 'off_farright': {'xalign': 1.6},
    'off_farleft': {'xalign': -0.73},
    'percsuperleft': {'xalign': 0.228, 'yalign': 0.7},
    'percrightcenter': {'xalign': 0.65, 'yalign': 0.7},
    'percfall': {'xalign': 0.45, 'yalign': 5.0},
    'percnotasright': {'xalign': 0.56, 'yalign': 0.7},
    'percentrist': {'xalign': 0.45, 'yalign': 0.7},
    'percleftstage': {'xalign': 0.456, 'yalign': 0.7},
    'truecenter': {'xalign': 0.5, 'yalign': 0.5},
    'center': {'xalign': 0.5, 'yalign': 1.0},
}
PROPS = {'xalign': 0, 'yalign': 1, 'alpha': 2, 'zoom': 3, 'xzoom': 4, 'rotate': 5}
CHANNELS = {'ambient': 0, 'music': 1, 'sfx': 2, 'vo': 3, 'sound': 2}
LAYERS = {'master': 0, 'screens': 1}

# ATL opcodes
A_SET, A_LINEAR, A_PAUSE, A_PARALLEL, A_IMAGE, A_END = 1, 2, 3, 4, 5, 0

# set by Compiler: maps an inline im.Flip("file") inside ATL to an image id
IMAGE_RESOLVER = None

def num(s):
    return float(s)

def parse_props(tokens, line):
    """['alpha','1.0','xalign','0.3'] or ['off_right'] -> dict."""
    out, i = {}, 0
    while i < len(tokens):
        t = tokens[i]
        if t in POSITIONS:
            out.update(POSITIONS[t]); i += 1
        elif t in PROPS:
            out[t] = num(tokens[i + 1]); i += 2
        elif t == 'size':   # transform_logo only (menu screen); ignore
            i += 3
        elif t == 'subpixel':
            i += 2
        else:
            raise SyntaxError(f'line {line}: unknown ATL property {t!r}')
    return out

def compile_atl(kids):
    """List of ATL nodes -> nested list program."""
    prog = []
    for n in kids:
        toks = n.text.replace('(', ' ').replace(')', ' ').replace(',', ' ').split()
        if n.text == 'parallel:':
            # consecutive parallel blocks are grouped
            if prog and prog[-1][0] == 'parallel' and prog[-1][2]:
                prog[-1][1].append(compile_atl(n.kids))
            else:
                prog.append(['parallel', [compile_atl(n.kids)], True])
            continue
        if prog and prog[-1][0] == 'parallel':
            prog[-1][2] = False
        if toks[0] == 'im.Flip':
            f = re.search(r'"([^"]+)"', n.text).group(1)
            prog.append(['image', IMAGE_RESOLVER(f, 'horizontal=True' in n.text)])
        elif toks[0] == 'linear':
            prog.append(['linear', num(toks[1]), parse_props(toks[2:], n.line)])
        elif toks[0] == 'easein':
            prog.append(['linear', num(toks[1]), parse_props(toks[2:], n.line)])
        elif toks[0] == 'pause':
            prog.append(['pause', num(toks[1])])
        elif re.match(r'^[0-9.]+$', toks[0]):
            prog.append(['pause', num(toks[0])])
        else:
            prog.append(['set', parse_props(toks, n.line)])
    return [p[:2] if p[0] == 'parallel' else p for p in prog]

def encode_atl(prog, out):
    for op in prog:
        if op[0] == 'set':
            out += struct.pack('>BB', A_SET, len(op[1]))
            for k, v in op[1].items():
                out += struct.pack('>Bh', PROPS[k], round(v * (10 if k == 'rotate' else 1000)))
        elif op[0] == 'linear':
            out += struct.pack('>BHB', A_LINEAR, round(op[1] * 1000), len(op[2]))
            for k, v in op[2].items():
                out += struct.pack('>Bh', PROPS[k], round(v * (10 if k == 'rotate' else 1000)))
        elif op[0] == 'image':
            out += struct.pack('>BH', A_IMAGE, op[1])
        elif op[0] == 'pause':
            out += struct.pack('>BH', A_PAUSE, round(op[1] * 1000))
        elif op[0] == 'parallel':
            out += struct.pack('>BB', A_PARALLEL, len(op[1]))
            for blk in op[1]:
                sub = bytearray()
                encode_atl(blk, sub)
                sub += bytes([A_END])
                out += struct.pack('>H', len(sub)) + sub
    return out

OP = dict(SCENE=1, SHOW=2, HIDE=3, PLAY=4, STOP=5, VOICE=6, SAY=7, MENU=8,
          JUMP=9, ENDING=10, PAUSE=11, RETURN=12, LABEL=13, TEXT=14)

# The game's texts are pictures of a Sidekick; on the real one they arrive as
# real texts. Transcribed from the images (and the voice lines that read them).
TEXTS = {
    'text crispin 1': ('571-777-4206', 'hey thanx 4 listening in the womens resterroom i love seeing '
                       'you and hereing youre voice ur attetute just works 4 me idk i wanted to '
                       'show you what u do to me'),
    'text crispin 2': ('571-777-4206', '[Picture message: CENSORED]'),
    'text colby': ('Coach Colby', "Hey honey just got out of jail, good news they're giving me my "
                   'job back so we can still hang out'),
    'text kylar 1': ('678-999-8212', '[Video message] Yo what\'s up this is Kylar, '
                     "let's break this gay ass photo lab! Fuck photography! It's for people who like animals!"),
}


class Compiler:
    def __init__(self, images_dir):
        self.strings, self.images, self.sounds = Table(), Table(), Table()
        self.chars, self.atls, self.tags = Table(), Table(), Table()
        self.labels = {}
        self.code = []          # list of (op, args) ; jumps resolved later
        self.image_files = self.scan_images(images_dir)
        self.image_defs = {}    # name -> {'file':..,'flip':bool} or movie
        self.char_names = {}
        self.texted = set()
        self.voice = None
        self.endings = []       # (id, sender str, preview str, body str)
        global IMAGE_RESOLVER
        IMAGE_RESOLVER = self.inline_image

    def inline_image(self, f, flip):
        name = os.path.splitext(f)[0].lower() + (' flipped' if flip else '')
        self.image_defs.setdefault(name, {'file': f, 'flip': flip})
        return self.images.get(name)

    @staticmethod
    def scan_images(d):
        out = {}
        for f in os.listdir(d):
            base, ext = os.path.splitext(f)
            if ext.lower() in ('.png', '.jpg', '.jpeg', '.webp'):
                out[base.lower()] = f
        return out

    def resolve_image(self, words, line):
        """Ren'Py image name lookup: longest attribute prefix that exists."""
        name = ' '.join(words).lower()
        if name in self.image_defs:
            return name
        if name in self.image_files:
            self.image_defs[name] = {'file': self.image_files[name], 'flip': False}
            return name
        # attributes after the tag are unordered in Ren'Py
        want = (words[0].lower(), frozenset(w.lower() for w in words[1:]))
        cands = [n for n in list(self.image_defs) + list(self.image_files)
                 if n.split()[0] == want[0] and frozenset(n.split()[1:]) == want[1]]
        if cands:
            n = cands[0]
            if n not in self.image_defs:
                self.image_defs[n] = {'file': self.image_files[n], 'flip': False}
            return n
        raise SyntaxError(f'line {line}: unknown image {name!r}')

    def emit(self, op, *args):
        self.code.append((op, args))

    def pause_of(self, s):
        m = re.search(r'with\s+Pause\s*\(\s*([0-9.]+)\s*\)', s)
        return float(m.group(1)) if m else 0.0

    def load_ending_tracker(self, screens_rpy):
        """The ending tracker's texting inbox lives in screens.rpy as three dict literals."""
        src = open(screens_rpy, encoding='utf-8').read()
        def grab(name):
            m = re.search(r'default ' + name + r' = (\{.*?\n    \})', src, re.S)
            return ast.literal_eval(m.group(1))
        names, details, texts = grab('endings_name'), grab('endings_details'), grab('ending_text')
        self.tip = self.strings.get(TRACKER_TIP)
        for eid in sorted(texts):
            self.endings.append((int(eid), self.strings.get(names[eid].strip()),
                                 self.strings.get(details[eid]), self.strings.get(texts[eid])))

    def toplevel(self, root):
        self.sounds.get('audio/NewTextAlert.mp3')
        for n in root.kids:
            t = n.text
            if t.startswith('define ') and 'Character(' in t:
                m = re.match(r'define (\w+) = Character\("([^"]*)"', t)
                self.char_names[m.group(1)] = m.group(2)
            elif t.startswith('image '):
                m = re.match(r'image (.+?) = (.*)$', t)
                name, rhs = m.group(1).lower(), m.group(2)
                if rhs.startswith('im.Flip'):
                    f = re.search(r'"([^"]+)"', rhs).group(1)
                    self.image_defs[name] = {'file': f, 'flip': True}
                elif rhs.startswith('Movie'):
                    f = re.search(r'play="([^"]+)"', rhs).group(1)
                    self.image_defs[name] = {'movie': f}
            elif t == 'init:':
                for k in n.kids:
                    if k.text.startswith('image black'):
                        self.image_defs['black'] = {'solid': [0, 0, 0]}
        self.image_defs.setdefault('white', {'solid': [255, 255, 255]})
        # engine UI assets, looked up by name
        for name, f in (('@menu_bg', 'gui/main_menu.png'), ('@logo', 'gui/ClassOf09logo.png'),
                        ('@nvl', 'gui/nvl.png')):
            self.image_defs[name] = {'file': f, 'flip': False}
            self.images.get(name)
        self.image_defs['@splash'] = {'movie': 'logosplash.webm'}
        self.images.get('@splash')
        self.sounds.get('audio/menu_piano.mp3')
        # UI sounds from screens.rpy: title menu buttons, story choices (PhoneSelect) and the texting inbox
        for ui in ('audio/MainMenuRollover.mp3', 'audio/MainMenuPress.mp3', 'audio/PhoneSelect.mp3', 'audio/Phone_slide.mp3',
                   'audio/Phone_button.mp3', 'audio/Phone_close.mp3'):
            self.sounds.get(ui)
        for n in root.kids:
            if n.text.startswith('label ') and n.text not in ('label splashscreen:',):
                name = n.text[6:-1]
                self.labels[name] = len(self.code)
                self.texted = set()
                self.emit('LABEL')
                self.block(n.kids)
            elif n.text == 'menu:':
                # the decompiler hoists a label-less menu after scene_0002's
                # body; it belongs to the preceding label (falls through).
                self.menu(n)

    def block(self, nodes):
        for n in nodes:
            self.stmt(n)

    def stmt(self, n):
        t = n.text
        if t.startswith('$ '):
            return self.python(t[2:].strip(), n)
        if t.startswith('scene'):
            return self.scene(n)
        if t.startswith('show screen') or t.startswith('hide screen'):
            return
        if t.startswith('show '):
            return self.show(n)
        if t.startswith('hide '):
            w = t[5:].rstrip(':').split()
            layer = 0
            if 'onlayer' in w:
                layer = LAYERS[w[w.index('onlayer') + 1]]; w = w[:w.index('onlayer')]
            return self.emit('HIDE', self.tags.get(w[0]), layer)
        if t.startswith('play '):
            m = re.match(r'play (\w+) "([^"]+)"(?: fadein ([0-9.]+))?', t)
            return self.emit('PLAY', CHANNELS[m.group(1)], self.sounds.get(m.group(2)),
                             round(float(m.group(3) or 0) * 1000))
        if t.startswith('stop '):
            m = re.match(r'stop (\w+)(?: fadeout ([0-9.]+))?', t)
            return self.emit('STOP', CHANNELS[m.group(1)], round(float(m.group(2) or 0) * 1000))
        if t.startswith('jump '):
            return self.emit('JUMP', t[5:].strip())
        if t == 'return':
            return self.emit('RETURN')
        if t == 'menu:':
            return self.menu(n)
        if t.startswith('if '):
            m = re.match(r'if "(\w+)" not in persistent.endings:', t)
            if not m:
                raise SyntaxError(f'line {n.line}: {t}')
            return self.emit('ENDING', int(m.group(1)[4:]))
        if t in ('window show', 'window hide') or t.startswith('with '):
            return
        raise SyntaxError(f'line {n.line}: unhandled {t!r}')

    def python(self, s, n):
        if s.startswith('setVoiceTrack('):
            self.voice = re.search(r'"([^"]+)"', s).group(1)
            self.emit('VOICE', self.sounds.get(self.voice))
        elif s.startswith('setWait('):
            a, b = re.match(r'setWait\(\s*([0-9.]+)\s*,\s*([0-9.]+)\s*\)', s).groups()
            self.wait = (round(float(a) * 1000), round(float(b) * 1000))
        elif s.startswith('speak('):
            m = re.match(r'speak\((\w+),\s*"(.*)"\s*\)$', s)
            who, text = m.group(1), m.group(2).replace('\\"', '"').replace('\\n', '\n')
            b, e = getattr(self, 'wait', (0, 0))
            self.emit('SAY', self.chars.get(self.char_names.get(who, who)),
                      self.strings.get(text), b, e)
        elif s.startswith('renpy.pause('):
            m = re.match(r'renpy\.pause\(\s*([0-9.]+)', s)
            self.emit('PAUSE', round(float(m.group(1)) * 1000))
        # quick_menu, persistent.*, _game_menu_screen, movie_cutscene: no-ops

    def split_show(self, t):
        head = t.rstrip(':')
        pause = self.pause_of(head)
        head = re.sub(r'\s+with\s+.*$', '', head)
        w = head.split()[1:]
        layer = 0
        if 'onlayer' in w:
            layer = LAYERS[w[w.index('onlayer') + 1]]; w = w[:w.index('onlayer')]
        at = None
        if 'at' in w:
            at = w[w.index('at') + 1:]; w = w[:w.index('at')]
        return w, layer, pause, at

    def atl_for(self, n, at):
        prog = []
        if at:
            prog.append(['set', parse_props(at, n.line)])
        prog += compile_atl(n.kids)
        if not prog:
            return 0xFFFF
        return self.atls.get(bytes(encode_atl(prog, bytearray()) + bytes([A_END])))

    def scene(self, n):
        w, layer, pause, at = self.split_show(n.text)
        if not w:
            return self.emit('SCENE', 0xFFFF, layer, 0xFFFF)
        img = self.images.get(self.resolve_image(w, n.line))
        self.emit('SCENE', img, layer, self.atl_for(n, at))
        if pause:
            self.emit('PAUSE', round(pause * 1000))

    def show(self, n):
        w, layer, pause, at = self.split_show(n.text)
        key = ' '.join(w).lower()
        if key in TEXTS and key not in self.texted:
            self.texted.add(key)
            sender, body = TEXTS[key]
            self.emit('TEXT', self.strings.get(sender), self.strings.get(body))
        img = self.images.get(self.resolve_image(w, n.line))
        self.emit('SHOW', self.tags.get(w[0]), img, layer, self.atl_for(n, at))
        if pause:
            self.emit('PAUSE', round(pause * 1000))

    def menu(self, n):
        choices = []
        for c in n.kids:
            if not c.text.startswith('"'):
                continue
            text = re.match(r'"(.*)":$', c.text).group(1).replace('\\n', '\n')
            target = None
            for k in c.kids:
                if k.text.startswith('jump '):
                    target = k.text[5:].strip()
            choices.append((self.strings.get(text), target))
        self.emit('MENU', choices)

    def assemble(self):
        # movies carry their audio as a same-named sound
        for name in self.images.items:
            if 'movie' in self.image_defs[name]:
                self.sounds.get(name)
        out = bytearray()
        offsets, fix = [], []
        for op, args in self.code:
            offsets.append(len(out))
            out.append(OP[op])
            if op == 'SCENE':
                out += struct.pack('>HBH', *args)
            elif op == 'SHOW':
                out += struct.pack('>HHBH', *args)
            elif op == 'HIDE':
                out += struct.pack('>HB', *args)
            elif op == 'PLAY':
                out += struct.pack('>BHH', *args)
            elif op == 'STOP':
                out += struct.pack('>BH', *args)
            elif op == 'VOICE':
                out += struct.pack('>H', *args)
            elif op == 'SAY':
                out += struct.pack('>BHII', *args)
            elif op == 'MENU':
                out.append(len(args[0]))
                for s, target in args[0]:
                    out += struct.pack('>H', s)
                    fix.append((len(out), target)); out += b'\0\0\0\0'
            elif op == 'JUMP':
                fix.append((len(out), args[0])); out += b'\0\0\0\0'
            elif op == 'ENDING':
                out.append(args[0])
            elif op == 'TEXT':
                out += struct.pack('>HH', *args)
            elif op == 'PAUSE':
                out += struct.pack('>I', *args)
        label_off = {k: (offsets[v] if v < len(offsets) else len(out)) for k, v in self.labels.items()}
        for pos, target in fix:
            out[pos:pos + 4] = struct.pack('>I', label_off[target])
        return bytes(out), label_off

def wstr(out, s):
    b = s.encode('utf-8')
    out += struct.pack('>H', len(b)) + b

def main():
    src, images_dir, out_dir = sys.argv[1:4]
    c = Compiler(images_dir)
    text = open(src, encoding='utf-8').read()
    c.load_ending_tracker(os.path.join(os.path.dirname(src), 'screens.rpy'))
    c.toplevel(parse_blocks(text.split('\n')))
    code, labels = c.assemble()

    out = bytearray(b'C9S1')
    out += struct.pack('>I', labels['start'])
    for tbl in (c.strings.items, c.chars.items):
        out += struct.pack('>H', len(tbl))
        for s in tbl:
            wstr(out, s)
    out += struct.pack('>H', len(c.images.items))
    for name in c.images.items:
        wstr(out, name)
    out += struct.pack('>H', len(c.sounds.items))
    for name in c.sounds.items:
        wstr(out, name)
    out += struct.pack('>H', len(c.atls.items))
    for a in c.atls.items:
        out += struct.pack('>H', len(a)) + a
    out.append(len(c.endings))
    for e in c.endings:
        out += struct.pack('>BHHH', *e)
    # the tracker's bonus: all 15 endings unlock scene_0999 ("UNKNOWN / VIDEO MESSAGE")
    out += struct.pack('>IH', labels.get('scene_0999', 0xFFFFFFFF), c.tip)
    out += struct.pack('>I', len(code)) + code
    # Auto Play's route (tools/auto_plan.py)
    segs = auto_plan.plan(bytes(code), labels['start'])
    out += auto_plan.encode(segs)

    os.makedirs(out_dir, exist_ok=True)
    open(os.path.join(out_dir, 'script.c9s'), 'wb').write(out)
    manifest = {
        'images': {n: c.image_defs[n] for n in c.images.items},
        'sounds': c.sounds.items,
        'movie_sounds': {n: c.image_defs[n]['movie'] for n in c.images.items if 'movie' in c.image_defs[n]},
        'labels': labels,
    }
    json.dump(manifest, open(os.path.join(out_dir, 'manifest.json'), 'w'), indent=1)
    json.dump([(op, [list(a) if isinstance(a, tuple) else a for a in args]) for op, args in c.code],
              open(os.path.join(out_dir, 'script.json'), 'w'))
    print(f'{len(c.code)} ops, {len(code)} bytes code, {len(out)} total; '
          f'{len(c.strings.items)} strings, {len(c.images.items)} images, '
          f'{len(c.sounds.items)} sounds, {len(c.atls.items)} ATL programs, '
          f'{len(labels)} labels, {len(segs)} auto play segments')

if __name__ == '__main__':
    main()
