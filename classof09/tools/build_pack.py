#!/usr/bin/env python3
"""Encode the game's media into C9I/C9A/C9V and write the C9P pack (see ../FORMATS.md).

Usage: build_pack.py game_dir build_dir
  build_dir must hold script.c9s + manifest.json from compile_script.py.
  Writes build_dir/classof09.c9p; per-asset blobs are cached in build_dir/cache/.

Env: ONLY=images|sounds|movies limits work (for iterating); JOBS=n parallelism.
"""
import hashlib, json, os, struct, subprocess, sys
from multiprocessing import Pool

import lz4.block
import numpy as np
from PIL import Image

# The 1920x1080 game scaled to the screen's 160px height is 284x160; the
# Sidekick shows the middle 240px of that (VIEW_W).
STAGE_H = 160
SCALE = STAGE_H / 1080
STAGE_W = round(1920 * SCALE)
VIEW_W = 240
# movies aren't cropped: full 16:9 width, centred vertically on the screen
MOVIE_W, MOVIE_H = 240, 136
KEY = 0x0020  # engine's transparent key in RGB16_BGR; real colours avoid it

AMB_MAX_S, AMB_XFADE_S = 20.0, 1.0
VIDEO_FPS = int(os.environ.get("C9V_FPS", 8))

def resolve(base, rel):
    """Case-insensitive path lookup (the script was written on Windows)."""
    path = base
    for part in rel.split('/'):
        cand = os.path.join(path, part)
        if not os.path.exists(cand):
            m = [x for x in os.listdir(path) if x.lower() == part.lower()]
            if not m:
                raise FileNotFoundError(os.path.join(base, rel))
            cand = os.path.join(path, m[0])
        path = cand
    return path

def c9z(raw):
    comp = lz4.block.compress(bytes(raw), mode='high_compression', store_size=False)
    return struct.pack('>II', len(raw), len(comp)) + comp

def bgr565(rgb):
    rgb = np.asarray(rgb, dtype=np.int32)
    v = ((rgb[..., 2] >> 3) << 11) | ((rgb[..., 1] >> 2) << 5) | (rgb[..., 0] >> 3)
    return np.where(v == KEY, 0, v).astype(np.uint16)

def from565(v):
    v = v.astype(np.int32)
    b, g, r = (v >> 11) & 31, (v >> 5) & 63, v & 31
    return np.stack([(r << 3) | (r >> 2), (g << 2) | (g >> 4), (b << 3) | (b >> 2)], -1)

def encode_image(args):
    game, images_dir, d = args
    if 'solid' in d:
        pal = bgr565(np.array([d['solid']]))
        return b'C9I1' + struct.pack('>HHBH', 1, 1, 2, 1) + pal.astype('>u2').tobytes() + c9z(b'\0')
    f = d['file']
    path = resolve(game, f) if '/' in f else resolve(images_dir, f)
    im = Image.open(path).convert('RGBA')
    w, h = max(1, round(im.width * SCALE)), max(1, round(im.height * SCALE))
    # premultiplied resize so transparent fringes don't go dark
    a = np.asarray(im, dtype=np.float32) / 255
    a[..., :3] *= a[..., 3:4]
    small = np.asarray(Image.fromarray((a * 255).astype(np.uint8)).resize((w, h), Image.LANCZOS),
                       dtype=np.float32) / 255
    alpha = small[..., 3]
    rgb = np.where(alpha[..., None] > 0.01, small[..., :3] / np.maximum(alpha[..., None], 1e-3), 0)
    rgb = (np.clip(rgb, 0, 1) * 255).astype(np.uint8)
    if d.get('flip'):
        rgb, alpha = rgb[:, ::-1], alpha[:, ::-1]
    opaque = alpha >= 0.5
    transparent = not opaque.all()
    ncol = 255 if transparent else 256
    q = Image.fromarray(rgb).quantize(colors=ncol, method=Image.Quantize.MEDIANCUT,
                                      dither=Image.Dither.FLOYDSTEINBERG)
    idx = np.asarray(q, dtype=np.uint8)
    palette = np.array(q.getpalette()[:3 * ncol], dtype=np.int32).reshape(-1, 3)
    used = int(idx.max()) + 1
    palette = palette[:used]
    if transparent:
        idx = np.where(opaque, idx + 1, 0).astype(np.uint8)
        palette = np.vstack([[0, 0, 0], palette])
    pal = bgr565(palette)
    return (b'C9I1' + struct.pack('>HHBH', w, h, 1 if transparent else 0, len(pal))
            + pal.astype('>u2').tobytes() + c9z(idx.tobytes()))

# All audio is MP3 for the device's native decoder (no Java decoding, and
# better quality per byte than the old 11 kHz IMA ADPCM). Scene voice tracks
# stay whole: they carry continuous room tone, and the engine plays them
# straight through, syncing the text to the playback position.

VOICE_RATE = MUSIC_RATE = MOVIE_RATE = AMB_RATE = 22050
VOICE_KBPS, AMB_KBPS, MOVIE_KBPS, MUSIC_KBPS = 32, 32, 32, 48

def ffmpeg_pcm(path, rate):
    raw = subprocess.run(['ffmpeg', '-v', 'error', '-i', path, '-ac', '1', '-ar', str(rate),
                          '-f', 's16le', '-'], capture_output=True, check=True).stdout
    return np.frombuffer(raw, dtype='<i2').astype(np.float32)

def mp3(pcm, rate, kbps):
    pcm = np.clip(pcm, -32768, 32767).astype('<i2').tobytes()
    return subprocess.run(['ffmpeg', '-v', 'error', '-f', 's16le', '-ar', str(rate), '-ac', '1',
                           '-i', '-', '-c:a', 'libmp3lame', '-b:a', f'{kbps}k', '-write_xing', '0',
                           '-id3v2_version', '0', '-f', 'mp3', '-'],
                          input=pcm, capture_output=True, check=True).stdout

def silence_mp3():
    return mp3(np.zeros(VOICE_RATE // 10, np.float32), VOICE_RATE, VOICE_KBPS)

def encode_sound(args):
    game, name, kind = args
    path = resolve(game, name)
    if kind == 'movie':
        has_audio = subprocess.run(['ffprobe', '-v', 'error', '-select_streams', 'a',
                                    '-show_entries', 'stream=index', '-of', 'csv=p=0', path],
                                   capture_output=True, text=True).stdout.strip()
        if not has_audio:
            return silence_mp3()
        return mp3(ffmpeg_pcm(path, MOVIE_RATE), MOVIE_RATE, MOVIE_KBPS)
    if kind == 'music':
        return mp3(ffmpeg_pcm(path, MUSIC_RATE) * 0.9, MUSIC_RATE, MUSIC_KBPS)
    if 'ambience' in name.lower():
        pcm = ffmpeg_pcm(path, AMB_RATE)
        n, x = int(AMB_MAX_S * AMB_RATE), int(AMB_XFADE_S * AMB_RATE)
        if len(pcm) > n + x:
            body, tail = pcm[:n].copy(), pcm[n:n + x]
            ramp = np.linspace(0, 1, x, dtype=np.float32)
            body[:x] = body[:x] * ramp + tail * (1 - ramp)
            pcm = body
        return mp3(pcm, AMB_RATE, AMB_KBPS)
    return mp3(ffmpeg_pcm(path, VOICE_RATE), VOICE_RATE, VOICE_KBPS)

# per-block modes (2 bits each)
M_SKIP, M_SOLID, M_TWO, M_EIGHT = 0, 1, 2, 3
SSE_OK = 16 * 3 * int(os.environ.get("C9V_OK", 100))      # per-block "good enough" squared error
SSE_SKIP = 16 * 3 * int(os.environ.get("C9V_SKIP", 30))  # unchanged blocks must be this close to skip
LUMA = np.array([0.299, 0.587, 0.114], np.float32)
# pixel indices of each 2x2 quadrant (TL, TR, BL, BR), row-major within the quadrant
QUAD = np.array([[0, 1, 4, 5], [2, 3, 6, 7], [8, 9, 12, 13], [10, 11, 14, 15]])

def q565(c):
    return from565(bgr565(np.clip(c, 0, 255))).astype(np.float32)

def two_colour(px):
    """px (..., n, 3) -> (mask (..., n) bool, c0, c1 quantised, recon)"""
    luma = px @ LUMA
    mask = luma > luma.mean(-1, keepdims=True)
    m = mask[..., None]
    n1 = mask.sum(-1, keepdims=True)
    c1 = (px * m).sum(-2) / np.maximum(n1, 1)
    c0 = (px * ~m).sum(-2) / np.maximum(px.shape[-2] - n1, 1)
    c0, c1 = q565(c0), q565(c1)
    recon = np.where(m, c1[..., None, :], c0[..., None, :])
    return mask, c0, c1, recon

def maskbits(mask):
    """(..., 16) bool -> (...,) u16, pixel 0 in the MSB"""
    return (mask.astype(np.uint32) << np.arange(15, -1, -1, dtype=np.uint32)).sum(-1).astype(np.uint16)

def movie_alpha_decoder(path):
    """libvpx decoder name if the movie is VP8/VP9 with an alpha channel, else None.
    ffmpeg's built-in VP8/VP9 decoders drop WebM alpha; libvpx keeps it."""
    out = subprocess.run(['ffprobe', '-v', 'error', '-select_streams', 'v:0', '-show_entries',
                          'stream=codec_name:stream_tags=alpha_mode', '-of', 'json', path],
                         capture_output=True, text=True).stdout
    st = (json.loads(out or '{}').get('streams') or [{}])[0]
    tags = {k.lower(): v for k, v in (st.get('tags') or {}).items()}
    if tags.get('alpha_mode') != '1':
        return None
    return {'vp8': 'libvpx', 'vp9': 'libvpx-vp9'}.get(st.get('codec_name'))

def encode_movie(args):
    """C9V2, or C9VA for movies with alpha: transparent pixels decode to KEY,
    which the engine's compositor skips (so overlays only cost their visible part)."""
    game, f = args[:2]
    alpha_dec = args[2] if len(args) > 2 else None
    pre = ['-c:v', alpha_dec] if alpha_dec else []
    raw = subprocess.run(['ffmpeg', '-v', 'error'] + pre + ['-i', resolve(game, f), '-vf',
                          f'fps={VIDEO_FPS},scale={MOVIE_W}:{MOVIE_H}:flags=area',
                          '-f', 'rawvideo', '-pix_fmt', 'rgba' if alpha_dec else 'rgb24', '-'],
                         capture_output=True, check=True).stdout
    ch = 4 if alpha_dec else 3
    frames = np.frombuffer(raw, np.uint8).reshape(-1, MOVIE_H, MOVIE_W, ch)
    by, bx = MOVIE_H // 4, MOVIE_W // 4
    nb = by * bx
    out = bytearray((b'C9VA' if alpha_dec else b'C9V2') +
                    struct.pack('>HHHI', MOVIE_W, MOVIE_H, VIDEO_FPS * 100, len(frames)))
    recon = np.zeros((nb, 16, 3), np.float32)
    recon_op = np.ones((nb, 16), bool)     # which reconstructed pixels are visible
    for fi, fr in enumerate(frames):
        blocks = fr.reshape(by, 4, bx, 4, ch).transpose(0, 2, 1, 3, 4).reshape(nb, 16, ch)
        cur = blocks[..., :3].astype(np.float32)
        op = blocks[..., 3] >= 128 if alpha_dec else np.ones((nb, 16), bool)
        solid = q565(cur.mean(1))
        r_solid = np.broadcast_to(solid[:, None], cur.shape)
        m2, a2, b2, r_two = two_colour(cur)
        q = cur[:, QUAD]                                   # (nb, 4, 4, 3)
        m8, a8, b8, r8q = two_colour(q)
        r_eight = np.empty_like(cur)
        r_eight[:, QUAD] = r8q
        sse = lambda r: ((cur - r) ** 2).sum((1, 2))
        e_skip = (((cur - recon) ** 2).sum(2) * op).sum(1)   # visible pixels only
        e_solid, e_two = sse(r_solid), sse(r_two)
        mode = np.full(nb, M_EIGHT, np.uint8)
        mode[e_two <= SSE_OK] = M_TWO
        mode[e_solid <= SSE_OK] = M_SOLID
        # transparency: fully clear blocks are a solid KEY; partly clear ones use
        # the 8-colour mode with KEY for each quadrant's clear pixels
        clear, part = ~op.any(1), op.any(1) & ~op.all(1)
        mode[clear] = M_SOLID
        mode[part] = M_EIGHT
        q_clear = q_part = np.zeros((nb, 4), bool)
        if part.any():
            qop = op[:, QUAD]                                  # (nb, 4, 4)
            qmean = (q * qop[..., None]).sum(2) / np.maximum(qop.sum(2), 1)[..., None]
            qmean = q565(qmean)
            q_clear, q_part = ~qop.any(2), qop.any(2) & ~qop.all(2)
            sel = part[:, None] & q_part
            a8 = a8.copy(); b8 = b8.copy(); m8 = m8.copy()
            b8[sel] = qmean[sel]
            m8[sel] = qop[sel]
            m8[part[:, None] & q_clear] = False
            r8q = np.where(m8[..., None], b8[:, :, None, :], a8[:, :, None, :])
            r_eight[:, QUAD] = r8q
        if fi:
            mode[(e_skip <= SSE_SKIP) & (recon_op == op).all(1)] = M_SKIP
        recon = np.where((mode == M_SOLID)[:, None, None], r_solid, recon)
        recon = np.where((mode == M_TWO)[:, None, None], r_two, recon)
        recon = np.where((mode == M_EIGHT)[:, None, None], r_eight, recon)
        recon_op = np.where((mode == M_SKIP)[:, None], recon_op, op)
        s_, t_, e_ = mode == M_SOLID, mode == M_TWO, mode == M_EIGHT
        packed = np.zeros((nb + 3) // 4, np.uint8)
        for k in range(4):
            sub = mode[k::4]
            packed[:len(sub)] |= sub << (6 - 2 * k)
        two = np.stack([bgr565(a2[t_]), bgr565(b2[t_]), maskbits(m2[t_])], 1)
        m8bits = maskbits(m8[e_].reshape(-1, 16))
        c0q, c1q = bgr565(a8), bgr565(b8)                      # (nb, 4)
        if alpha_dec:
            c0q[part[:, None] & (q_part | q_clear)] = KEY
            c1q[part[:, None] & q_clear] = KEY
        cols = np.stack([c0q[e_], c1q[e_]], 2).reshape(-1, 8)
        solid_vals = bgr565(solid)
        solid_vals[clear] = KEY
        eight = np.concatenate([cols, m8bits[:, None]], 1)
        payload = (packed.tobytes() + struct.pack('>HHH', int(s_.sum()), int(t_.sum()), int(e_.sum()))
                   + solid_vals[s_].astype('>u2').tobytes()
                   + two.astype('>u2').tobytes() + eight.astype('>u2').tobytes())
        out += c9z(payload)
    return bytes(out)

MOVIE_KEY = f'mov2-{SSE_OK}-{SSE_SKIP}-{VIDEO_FPS}'
IMAGE_KEY = f'img-{STAGE_H}'

def cached(cache, key, fn, args):
    h = hashlib.sha1((key + json.dumps(args[1:], sort_keys=True, default=str)).encode()).hexdigest()[:16]
    p = os.path.join(cache, h)
    if os.path.exists(p):
        return open(p, 'rb').read()
    data = fn(args)
    open(p + '.tmp', 'wb').write(data)
    os.replace(p + '.tmp', p)
    return data

def job(a):
    cache, key, fn, args = a
    data = cached(cache, key, globals()[fn], args)
    return len(data)

def main():
    game, build = sys.argv[1:3]
    images_dir = os.path.join(game, 'images')
    man = json.load(open(os.path.join(build, 'manifest.json')))
    cache = os.path.join(build, 'cache')
    os.makedirs(cache, exist_ok=True)
    only = os.environ.get('ONLY')
    jobs = []
    img_jobs, snd_jobs = [], []
    for name, d in man['images'].items():
        if 'movie' in d:
            # alpha movies (overlays like the pill pop) add the decoder to the args,
            # so only they get a new cache entry
            dec = movie_alpha_decoder(resolve(game, d['movie']))
            a = (MOVIE_KEY, 'encode_movie', (game, d['movie']) + ((dec,) if dec else ()))
        else:
            a = (IMAGE_KEY, 'encode_image', (game, images_dir, d))
        img_jobs.append(a)
    for name in man['sounds']:
        if name in man['movie_sounds']:
            a = ('snd-mp3b', 'encode_sound', (game, man['movie_sounds'][name], 'movie'))
        elif name == 'audio/menu_piano.mp3':
            a = ('snd-mp3b', 'encode_sound', (game, name, 'music'))
        else:
            a = ('snd-mp3b', 'encode_sound', (game, name, 'auto'))
        snd_jobs.append(a)
    sel = {'images': [a for a in img_jobs if a[0] == IMAGE_KEY],
           'movies': [a for a in img_jobs if a[0] == MOVIE_KEY],
           'sounds': snd_jobs}
    todo = sel[only] if only else img_jobs + snd_jobs
    # movies first: they are the slowest
    todo = sorted(todo, key=lambda a: a[0] != MOVIE_KEY)
    with Pool(int(os.environ.get('JOBS', os.cpu_count()))) as pool:
        for i, n in enumerate(pool.imap_unordered(job, [(cache,) + a for a in todo])):
            if i % 25 == 0:
                print(f'  {i + 1}/{len(todo)}', flush=True)
    if only:
        return
    # Split by how often things change, so a typical update only means copying
    # the small index file to the card (see tools/sync.sh). Segment contents are
    # deterministic: unchanged categories come out byte-identical.
    INDEX, IMAGES, MOVIES, VOICE, AUDIO = 0, 1, 2, 3, 4
    files = ['', 'images.c9p', 'movies.c9p', 'voice.c9p', 'audio.c9p']

    def segment_of_image(a):
        return MOVIES if a[0] == MOVIE_KEY else IMAGES

    def segment_of_sound(name):
        if name in man['movie_sounds']:
            return MOVIES
        if name.lower().startswith('audio/scenes/'):
            return VOICE
        if 'ambience' in name.lower() or name == 'audio/menu_piano.mp3':
            return AUDIO
        return INDEX   # small UI sounds

    entries = [(INDEX, open(os.path.join(build, 'script.c9s'), 'rb').read())]
    entries += [(segment_of_image(a), cached(cache, a[0], globals()[a[1]], a[2])) for a in img_jobs]
    entries += [(segment_of_sound(n), cached(cache, a[0], globals()[a[1]], a[2]))
                for n, a in zip(man['sounds'], snd_jobs)]

    names = bytearray()
    for f in files:
        names += bytes([len(f)]) + f.encode()
    head = b'C9P3' + struct.pack('>HHHHB', 1, len(img_jobs), len(snd_jobs), 0, len(files)) + names
    seg = [bytearray() for _ in files]
    index_off = len(head) + 9 * len(entries)
    table = bytearray()
    for f, b in entries:
        off = index_off + len(seg[INDEX]) if f == INDEX else len(seg[f])
        table += struct.pack('>BII', f, off, len(b))
        seg[f] += b
    seg[INDEX] = head + table + seg[INDEX]
    for f, data in zip(files, seg):
        path = os.path.join(build, f or 'classof09.c9p')
        if not os.path.exists(path) or open(path, 'rb').read() != data:
            open(path + '.tmp', 'wb').write(data)
            os.replace(path + '.tmp', path)
    print('pack:', {(f or 'classof09.c9p'): f'{len(d) / 1e6:.1f}MB' for f, d in zip(files, seg)})

if __name__ == '__main__':
    main()
