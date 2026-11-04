package com.redanger.classof09;

/**
 * Runs one ATL program (see FORMATS.md) against a sprite. Timed statements
 * chain on their scheduled end times, not on when the tick noticed, so long
 * animations stay in sync with the voice track.
 */
final class Atl {
	private static final int SET = 1, LINEAR = 2, PAUSE = 3, PARALLEL = 4, IMAGE = 5;

	private final Stage mStage;
	private final Sprite mSprite;
	private final byte[] mP;
	private int mPc;
	private final int mEnd;
	private long mT;            // when the current statement started
	private int mOp, mDur, mN;  // current timed statement
	private final int[] mWhich = new int[6], mFrom = new int[6], mTo = new int[6];
	private Atl[] mKids;
	boolean done;
	long endTime;

	Atl(Stage stage, Sprite s, byte[] prog, int pc, int end, long start) {
		mStage = stage;
		mSprite = s;
		mP = prog;
		mPc = pc;
		mEnd = end;
		mT = start;
	}

	/** Advances to `now`. Returns true if anything visible changed. */
	boolean advance(long now) {
		boolean changed = false;
		int[] prop = mSprite.prop;
		while (!done) {
			if (mOp != 0) {
				if (mOp == PARALLEL) {
					boolean all = true;
					long end = mT;
					for (int i = 0; i < mKids.length; i++) {
						changed |= mKids[i].advance(now);
						if (!mKids[i].done) all = false;
						else if (mKids[i].endTime > end) end = mKids[i].endTime;
					}
					if (!all) return changed;
					mT = end;
				} else {
					long e = now - mT;
					if (mOp == LINEAR) {
						int f = e >= mDur ? 1000 : (int) (e * 1000 / (mDur == 0 ? 1 : mDur));
						for (int i = 0; i < mN; i++) {
							prop[mWhich[i]] = mFrom[i] + (mTo[i] - mFrom[i]) * f / 1000;
						}
						changed = true;
					}
					if (e < mDur) return changed;
					mT += mDur;
				}
				mOp = 0;
			}
			if (mPc >= mEnd || mP[mPc] == 0) {
				done = true;
				endTime = mT;
				break;
			}
			int op = mP[mPc++];
			switch (op) {
			case SET: {
				int n = mP[mPc++];
				for (int i = 0; i < n; i++, mPc += 3) prop[mP[mPc]] = Pack.s16(mP, mPc + 1);
				changed = true;
				break;
			}
			case LINEAR: {
				mDur = Pack.u16(mP, mPc);
				mN = mP[mPc + 2];
				mPc += 3;
				for (int i = 0; i < mN; i++, mPc += 3) {
					mWhich[i] = mP[mPc];
					mFrom[i] = prop[mP[mPc]];
					mTo[i] = Pack.s16(mP, mPc + 1);
				}
				mOp = LINEAR;
				break;
			}
			case PAUSE:
				mDur = Pack.u16(mP, mPc);
				mPc += 2;
				mOp = PAUSE;
				break;
			case PARALLEL: {
				int n = mP[mPc++];
				mKids = new Atl[n];
				for (int i = 0; i < n; i++) {
					int len = Pack.u16(mP, mPc);
					mKids[i] = new Atl(mStage, mSprite, mP, mPc + 2, mPc + 2 + len, mT);
					mPc += 2 + len;
				}
				mOp = PARALLEL;
				break;
			}
			case IMAGE:
				mStage.setImage(mSprite, Pack.u16(mP, mPc));
				mPc += 2;
				changed = true;
				break;
			default:
				done = true;
			}
		}
		return changed;
	}
}
