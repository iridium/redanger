package com.redanger.classof09;

import java.io.IOException;

/** Plays a C9V2/C9VA movie: per 4x4 block, skip / solid / 2-colour / 8-colour (see FORMATS.md). */
final class Video {
	private final Pack mPack;
	private final int mEntry;
	private final int mFps100, mFrames, mBw, mBlocks;
	private final byte[] mHead = new byte[8];
	private byte[] mComp = new byte[4096], mRaw = new byte[8192];
	private int mPos, mFrame = -1;
	private long mStart = -1;
	final Img img;

	Video(Pack pack, int entry, byte[] head) {
		mPack = pack;
		mEntry = entry;
		int w = Pack.u16(head, 4), h = Pack.u16(head, 6);
		mFps100 = Pack.u16(head, 8);
		mFrames = Pack.s32(head, 10);
		mBw = w / 4;
		mBlocks = mBw * (h / 4);
		mPos = 14;
		// "C9VA" movies carry alpha as the colour key: clear pixels decode to
		// Img.KEY, which the compositor skips.
		boolean alpha = head[3] == 'A';
		short[] px = new short[w * h];
		if (alpha) for (int i = 0; i < px.length; i++) px[i] = Img.KEY;
		img = new Img(w, h, px, alpha, false);
	}

	boolean done() { return mFrame >= mFrames - 1; }

	/** Advances to the frame due at `now`; returns true if the picture changed. */
	boolean update(long now) {
		if (mStart < 0) mStart = now;
		int want = (int) ((now - mStart) * mFps100 / 100000);
		if (want >= mFrames) want = mFrames - 1;
		boolean changed = false;
		try {
			while (mFrame < want) {
				decodeFrame();
				changed = true;
			}
		} catch (IOException e) {
			mFrame = mFrames;
		}
		return changed;
	}

	private void decodeFrame() throws IOException {
		mPack.read(mEntry, mPos, mHead, 0, 8);
		int raw = Pack.s32(mHead, 0), comp = Pack.s32(mHead, 4);
		if (comp > mComp.length) mComp = new byte[comp];
		if (raw > mRaw.length) mRaw = new byte[raw];
		mPack.read(mEntry, mPos + 8, mComp, 0, comp);
		mPos += 8 + comp;
		mFrame++;
		Lz4.decode(mComp, 0, comp, mRaw);
		byte[] p = mRaw;
		int m = (mBlocks + 3) / 4;
		int ns = Pack.u16(p, m), n2 = Pack.u16(p, m + 2);
		int sp = m + 6, tp = sp + ns * 2, ep = tp + n2 * 6;
		short[] px = img.px;
		int w = img.w;
		for (int blk = 0; blk < mBlocks; blk++) {
			int mode = (p[blk >> 2] >> (6 - 2 * (blk & 3))) & 3;
			if (mode == 0) continue;
			int o = (blk / mBw) * 4 * w + (blk % mBw) * 4;
			if (mode == 1) {
				short c = Img.color(Pack.u16(p, sp));
				sp += 2;
				for (int y = 0; y < 4; y++, o += w) {
					px[o] = c; px[o + 1] = c; px[o + 2] = c; px[o + 3] = c;
				}
			} else if (mode == 2) {
				short c0 = Img.color(Pack.u16(p, tp)), c1 = Img.color(Pack.u16(p, tp + 2));
				int bits = Pack.u16(p, tp + 4);
				tp += 6;
				for (int y = 0; y < 4; y++, o += w, bits <<= 4) {
					px[o] = (bits & 0x8000) != 0 ? c1 : c0;
					px[o + 1] = (bits & 0x4000) != 0 ? c1 : c0;
					px[o + 2] = (bits & 0x2000) != 0 ? c1 : c0;
					px[o + 3] = (bits & 0x1000) != 0 ? c1 : c0;
				}
			} else {
				int bits = Pack.u16(p, ep + 16);
				for (int q = 0; q < 4; q++) {
					short c0 = Img.color(Pack.u16(p, ep + q * 4)), c1 = Img.color(Pack.u16(p, ep + q * 4 + 2));
					int b = bits >> (12 - 4 * q);
					int qo = o + (q >> 1) * 2 * w + (q & 1) * 2;
					px[qo] = (b & 8) != 0 ? c1 : c0;
					px[qo + 1] = (b & 4) != 0 ? c1 : c0;
					px[qo + w] = (b & 2) != 0 ? c1 : c0;
					px[qo + w + 1] = (b & 1) != 0 ? c1 : c0;
				}
				ep += 18;
			}
		}
	}
}
