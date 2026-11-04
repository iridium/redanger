#!/usr/bin/env python3
"""Launcher icons and splash for the app bundle, from the game's logo.

Usage: make_icons.py game_dir assets_dir
"""
import os, sys
from PIL import Image

def fit(im, w, h, bg=None, scale=1.0):
    im = im.crop(im.getbbox())
    r = min(w / im.width, h / im.height) * scale
    im = im.resize((max(1, round(im.width * r)), max(1, round(im.height * r))), Image.LANCZOS)
    c = Image.new('RGBA', (w, h), bg or (0, 0, 0, 0))
    c.alpha_composite(im, ((w - im.width) // 2, (h - im.height) // 2))
    return c

game, out = sys.argv[1:3]
os.makedirs(out, exist_ok=True)
logo = Image.open(os.path.join(game, 'gui', 'ClassOf09logo.png')).convert('RGBA')
# native icons are small glyphs with room around them, not edge-to-edge art
fit(logo, 32, 32, scale=0.72).save(os.path.join(out, 'LargeIcon.png'))
fit(logo, 16, 16, scale=0.9).save(os.path.join(out, 'SmallIcon.png'))

# Games-screen preview (ID_SPLASH_SCREEN_NO_TITLE): the full 148x136 area,
# transparent except the logo, which the launcher overlays on its background.
fit(logo, 148, 136, scale=0.95).save(os.path.join(out, 'Splash.png'))
