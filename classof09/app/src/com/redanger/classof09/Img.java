package com.redanger.classof09;

/** A decoded picture in screen pixel format. Transparent pixels hold KEY. */
final class Img {
	static final short KEY = 0x0020;
	/** True when the screen is RGB16 (r high) rather than the M2's RGB16_BGR. */
	static boolean sSwapRB;

	final int w, h;
	final short[] px;
	final boolean transparent;
	/** Solid images fill the whole stage with px[0]. */
	final boolean solid;

	Img(int w, int h, short[] px, boolean transparent, boolean solid) {
		this.w = w;
		this.h = h;
		this.px = px;
		this.transparent = transparent;
		this.solid = solid;
	}

	static short color(int v) {
		if (sSwapRB) v = ((v & 0x1F) << 11) | (v & 0x7E0) | ((v >> 11) & 0x1F);
		return (short) v;
	}

	/** Decodes a C9I blob. */
	static Img decode(byte[] b) {
		int w = Pack.u16(b, 4), h = Pack.u16(b, 6), flags = b[8] & 0xFF, n = Pack.u16(b, 9);
		short[] pal = new short[256];
		for (int i = 0; i < n; i++) pal[i] = color(Pack.u16(b, 11 + i * 2));
		boolean trans = (flags & 1) != 0;
		if (trans) pal[0] = KEY;
		byte[] idx = Pack.unz(b, 11 + n * 2);
		short[] px = new short[w * h];
		for (int i = 0; i < px.length; i++) px[i] = pal[idx[i] & 0xFF];
		return new Img(w, h, px, trans, (flags & 2) != 0);
	}
}
