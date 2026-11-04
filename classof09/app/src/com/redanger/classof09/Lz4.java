package com.redanger.classof09;

/** LZ4 block decoder (C9Z payloads). */
final class Lz4 {
	private Lz4() {}

	/** Decodes src[off, off+len) into dst[0, rawLen); returns rawLen. */
	static int decode(byte[] src, int off, int len, byte[] dst) {
		int s = off, end = off + len, d = 0;
		while (s < end) {
			int token = src[s++] & 0xFF;
			int lit = token >>> 4;
			if (lit == 15) {
				int b;
				do { b = src[s++] & 0xFF; lit += b; } while (b == 255);
			}
			System.arraycopy(src, s, dst, d, lit);
			s += lit;
			d += lit;
			if (s >= end) break;
			int moff = (src[s] & 0xFF) | ((src[s + 1] & 0xFF) << 8);
			s += 2;
			int mlen = token & 15;
			if (mlen == 15) {
				int b;
				do { b = src[s++] & 0xFF; mlen += b; } while (b == 255);
			}
			mlen += 4;
			int from = d - moff;
			if (moff >= mlen) {
				System.arraycopy(dst, from, dst, d, mlen);
				d += mlen;
			} else {
				for (int i = 0; i < mlen; i++) dst[d++] = dst[from++];
			}
		}
		return d;
	}
}
