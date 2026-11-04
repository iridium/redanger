package com.redanger.classof09;

import java.io.IOException;
import java.util.Hashtable;
import java.util.Vector;

/** Ren'Py-style layers of sprites, composited in software into a framebuffer. */
final class Stage {
	/**
	 * The 1920x1080 game scaled to the screen's 160px height is 284x160 (W x H):
	 * sprites are placed on that virtual stage, as in Ren'Py, and the screen
	 * shows its middle VW pixels (offset OX).
	 */
	static final int W = 284, H = 160, VW = 240, OX = (W - VW) / 2;
	private static final int CACHE_PIXELS = 400000;

	interface Listener {
		void movieStarted(Sprite s, int imgId);
	}

	private final Pack mPack;
	private final byte[][] mAtl;
	private final Listener mListener;
	private final Vector[] mLayers = { new Vector(), new Vector() };
	private final Hashtable mCache = new Hashtable();
	private int mCachePixels;
	boolean dirty = true;
	/**
	 * Layer 0 (backgrounds, characters) composited on its own, redrawn only when
	 * something on it changes. Overlays on layer 1 (e.g. the pill-pop movie) then
	 * cost a row copy plus their own pixels instead of a whole recomposite.
	 */
	private final short[] mBase = new short[VW * H];
	private boolean mBaseDirty = true;

	Stage(Pack pack, byte[][] atl, Listener l) {
		mPack = pack;
		mAtl = atl;
		mListener = l;
	}

	void scene(int img, int layer, int atl, long now) {
		clear(layer);
		if (layer == 0) mBaseDirty = true;
		if (img != 0xFFFF) show(-1 - img, img, layer, atl, now);
	}

	void show(int tag, int img, int layer, int atl, long now) {
		Vector v = mLayers[layer];
		Sprite s = find(v, tag);
		if (s == null) {
			s = new Sprite(tag);
			v.addElement(s);
		}
		setImage(s, img);
		if (layer == 0) mBaseDirty = true;
		if (atl != 0xFFFF) {
			byte[] p = mAtl[atl];
			s.atl = new Atl(this, s, p, 0, p.length, now);
			s.atl.advance(now);
		}
		dirty = true;
	}

	void hide(int tag, int layer) {
		Vector v = mLayers[layer];
		Sprite s = find(v, tag);
		if (s != null) v.removeElement(s);
		if (layer == 0) mBaseDirty = true;
		dirty = true;
	}

	void clear(int layer) {
		mLayers[layer].removeAllElements();
		if (layer == 0) mBaseDirty = true;
		dirty = true;
	}

	void clearAll() {
		clear(0);
		clear(1);
	}

	void setImage(Sprite s, int img) {
		if (s.imgId == img && s.img != null) return;
		s.imgId = img;
		s.video = null;
		try {
			int e = mPack.imageEntry(img);
			byte[] head = new byte[14];
			mPack.read(e, 0, head, 0, Math.min(14, mPack.length(e)));
			if (head[2] == 'V') {
				s.video = new Video(mPack, e, head);
				s.img = s.video.img;
				mListener.movieStarted(s, img);
			} else {
				s.img = load(img);
			}
		} catch (IOException ex) {
			s.img = null;
		}
		dirty = true;
	}

	private Img load(int img) throws IOException {
		Integer key = new Integer(img);
		Img i = (Img) mCache.get(key);
		if (i != null) return i;
		i = Img.decode(mPack.read(mPack.imageEntry(img)));
		if (mCachePixels + i.px.length > CACHE_PIXELS) {
			mCache.clear();
			mCachePixels = 0;
		}
		mCache.put(key, i);
		mCachePixels += i.px.length;
		return i;
	}

	/** The sprite with this tag on a layer, or null. */
	Sprite sprite(int tag, int layer) {
		return find(mLayers[layer], tag);
	}

	private static Sprite find(Vector v, int tag) {
		for (int i = 0; i < v.size(); i++) {
			Sprite s = (Sprite) v.elementAt(i);
			if (s.tag == tag) return s;
		}
		return null;
	}

	/** Sprite currently showing a movie, if any. */
	Sprite movie() {
		for (int l = 0; l < 2; l++) {
			Vector v = mLayers[l];
			for (int i = 0; i < v.size(); i++) {
				Sprite s = (Sprite) v.elementAt(i);
				if (s.video != null) return s;
			}
		}
		return null;
	}

	/** Runs transforms and movies; returns true if a recomposite is needed. */
	boolean tick(long now) {
		boolean changed = dirty;
		for (int l = 0; l < 2; l++) {
			Vector v = mLayers[l];
			for (int i = 0; i < v.size(); i++) {
				Sprite s = (Sprite) v.elementAt(i);
				boolean c = false;
				if (s.atl != null) {
					c = s.atl.advance(now);
					if (s.atl.done) s.atl = null;
				}
				if (s.video != null) c |= s.video.update(now);
				if (c && l == 0) mBaseDirty = true;
				changed |= c;
			}
		}
		dirty = false;
		return changed;
	}

	/** Snaps every running transform to its end state (after a fast-forward). */
	void finishAnimations(long now) {
		for (int l = 0; l < 2; l++) {
			Vector v = mLayers[l];
			for (int i = 0; i < v.size(); i++) {
				Sprite s = (Sprite) v.elementAt(i);
				if (s.atl == null) continue;
				s.atl.advance(now + 600000);
				if (s.atl.done) s.atl = null;
			}
		}
		mBaseDirty = true;
		dirty = true;
	}

	void composite(short[] fb, int stride) {
		if (mBaseDirty) {
			for (int i = 0; i < mBase.length; i++) mBase[i] = 0;
			Vector v = mLayers[0];
			for (int i = 0; i < v.size(); i++) draw((Sprite) v.elementAt(i), mBase, VW);
			mBaseDirty = false;
		}
		for (int y = 0; y < H; y++) System.arraycopy(mBase, y * VW, fb, y * stride, VW);
		Vector v = mLayers[1];
		for (int i = 0; i < v.size(); i++) draw((Sprite) v.elementAt(i), fb, stride);
	}

	private static void draw(Sprite s, short[] fb, int stride) {
		Img img = s.img;
		if (img == null) return;
		int[] p = s.prop;
		int a = p[Sprite.ALPHA] * 32 / 1000;
		if (a <= 0) return;
		if (a > 32) a = 32;
		if (img.solid) {
			fill(fb, stride, img.px[0], a);
			return;
		}
		int xz = p[Sprite.XZOOM];
		int zx = (int) ((long) p[Sprite.ZOOM] * Math.abs(xz) / 1000);
		int zy = p[Sprite.ZOOM];
		if (zx <= 0 || zy <= 0) return;
		// size and position in 1/1000 px: a slow zoom or pan then moves smoothly, rather than
		// the size and the offset each snapping to whole pixels on different frames
		long dwm = (long) img.w * zx, dhm = (long) img.h * zy;
		long xm = p[Sprite.XALIGN] * (W * 1000L - dwm) / 1000 - OX * 1000L;
		long ym = p[Sprite.YALIGN] * (H * 1000L - dhm) / 1000;
		if (s.video != null) {
			// movies aren't cropped: full width, centred vertically on the screen
			xm = (VW * 1000L - dwm) / 2;
			ym = (H * 1000L - dhm) / 2;
		}
		if (zx == 1000 && zy == 1000) {   // unscaled art sits on whole pixels, as before
			xm = fdiv(xm, 1000) * 1000;
			ym = fdiv(ym, 1000) * 1000;
		}
		int dw = (int) (dwm / 1000), dh = (int) (dhm / 1000);
		if (dw <= 0 || dh <= 0) return;
		int x = (int) fdiv(xm, 1000), y = (int) fdiv(ym, 1000);
		if (p[Sprite.ROTATE] % 3600 != 0) {
			drawRotated(img, fb, stride, x, y, dw, dh, p[Sprite.ROTATE], a);
			return;
		}
		// the pixels whose centres fall on the image
		int x0 = (int) Math.max(fdiv(xm + 499, 1000), 0), x1 = (int) Math.min(fdiv(xm + dwm + 499, 1000), VW);
		int y0 = (int) Math.max(fdiv(ym + 499, 1000), 0), y1 = (int) Math.min(fdiv(ym + dhm + 499, 1000), H);
		if (x0 >= x1 || y0 >= y1) return;
		boolean flip = xz < 0;
		short[] src = img.px;
		int sw = img.w, sh = img.h;
		if (dw == sw && dh == sh && !flip && a == 32) {
			for (int dy = y0; dy < y1; dy++) {
				int so = (dy - y) * sw + (x0 - x), d = dy * stride + x0, n = x1 - x0;
				if (!img.transparent) {
					System.arraycopy(src, so, fb, d, n);
				} else {
					for (int k = 0; k < n; k++) {
						short c = src[so + k];
						if (c != Img.KEY) fb[d + k] = c;
					}
				}
			}
			return;
		}
		// sample the source under each destination pixel's centre, in 16.16
		int stepX = (int) ((1000L << 16) / zx);
		int fx0 = (int) ((((long) x0 * 1000 + 500 - xm) << 16) / zx);
		for (int dy = y0; dy < y1; dy++) {
			int sy = (int) ((((long) dy * 1000 + 500 - ym) << 16) / zy >> 16);
			if (sy < 0) sy = 0;
			else if (sy >= sh) sy = sh - 1;
			int row = sy * sw;
			int fx = fx0;
			int d = dy * stride + x0;
			for (int dx = x0; dx < x1; dx++, fx += stepX, d++) {
				int sx = fx >> 16;
				if (sx < 0) sx = 0;
				else if (sx >= sw) sx = sw - 1;
				if (flip) sx = sw - 1 - sx;
				short c = src[row + sx];
				if (c == Img.KEY) continue;
				fb[d] = a == 32 ? c : blend(c, fb[d], a);
			}
		}
	}

	/** a / b rounded down (b > 0). */
	private static long fdiv(long a, long b) {
		return a >= 0 ? a / b : -((-a + b - 1) / b);
	}

	private static void drawRotated(Img img, short[] fb, int stride, int x, int y, int dw, int dh, int deci, int a) {
		double r = -deci * Math.PI / 1800;
		int cos = (int) (Math.cos(r) * 65536), sin = (int) (Math.sin(r) * 65536);
		int cx = x + dw / 2, cy = y + dh / 2;
		int rad = (int) Math.sqrt(dw * dw + dh * dh) / 2 + 1;
		int sx = (img.w << 16) / dw, sy = (img.h << 16) / dh;
		for (int py = Math.max(cy - rad, 0); py < Math.min(cy + rad, H); py++) {
			for (int px = Math.max(cx - rad, 0); px < Math.min(cx + rad, VW); px++) {
				int ux = px - cx, uy = py - cy;
				int rx = ((ux * cos - uy * sin) >> 16) + dw / 2;
				int ry = ((ux * sin + uy * cos) >> 16) + dh / 2;
				if (rx < 0 || ry < 0 || rx >= dw || ry >= dh) continue;
				short c = img.px[((ry * sy) >> 16) * img.w + ((rx * sx) >> 16)];
				if (c == Img.KEY) continue;
				int d = py * stride + px;
				fb[d] = a == 32 ? c : blend(c, fb[d], a);
			}
		}
	}

	private static void fill(short[] fb, int stride, short c, int a) {
		for (int y = 0; y < H; y++) {
			int o = y * stride;
			if (a == 32) {
				for (int x = 0; x < VW; x++) fb[o + x] = c;
			} else {
				for (int x = 0; x < VW; x++) fb[o + x] = blend(c, fb[o + x], a);
			}
		}
	}

	/** 565 blend of src over dst, a in 0..32. Works for RGB and BGR order alike. */
	static short blend(int s, int d, int a) {
		s &= 0xFFFF;
		d &= 0xFFFF;
		s = (s | (s << 16)) & 0x07E0F81F;
		d = (d | (d << 16)) & 0x07E0F81F;
		d = (d + (((s - d) * a) >> 5)) & 0x07E0F81F;
		return (short) (d | (d >>> 16));
	}
}
