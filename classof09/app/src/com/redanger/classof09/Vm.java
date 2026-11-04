package com.redanger.classof09;

/** Interprets C9S bytecode (see FORMATS.md). */
final class Vm {
	static final int RUN = 0, SAY = 1, PAUSE = 2, MENU = 3, ENDED = 4, STOPPED = 5;
	private static final int OP_SCENE = 1, OP_SHOW = 2, OP_HIDE = 3, OP_PLAY = 4, OP_STOP = 5,
		OP_VOICE = 6, OP_SAY = 7, OP_MENU = 8, OP_JUMP = 9, OP_ENDING = 10, OP_PAUSE = 11,
		OP_RETURN = 12, OP_LABEL = 13, OP_TEXT = 14;

	interface Host {
		void autosave(int pc, int ambient);
		void ending(int id);
	}

	private final Script mS;
	private final Stage mStage;
	private final Audio mAudio;
	private final Host mHost;
	private int mPc;
	private long mUntil;
	private int mLineEnd;
	// the current line's voice span and start time, for paging long text
	private int mLineBegin, mLineLen;
	private long mSayAt;
	int state = ENDED;

	// what the view shows
	String name, text;
	String[] choices;
	private int[] mTargets;
	int sel;
	/** Bytecode offset of the menu being shown (identifies it for auto mode). */
	int menuPc;
	boolean textChanged;
	/**
	 * Fast-forward (auto mode jumping back to a choice): lines and pauses don't
	 * wait, voice and story texts are skipped, and ambience is applied once at
	 * the end. Scene and sprite changes still run so the picture is right. It
	 * stops at the next menu, with animations snapped to where they end.
	 */
	boolean fast;
	private int mFastAmb;
	/**
	 * Auto mode: stop (state STOPPED) on reaching this bytecode offset while
	 * playing normally; -1 for none. Reset by start().
	 */
	int stopPc = -1;

	Vm(Script s, Stage stage, Audio audio, Host host) {
		mS = s;
		mStage = stage;
		mAudio = audio;
		mHost = host;
	}

	void start(int pc, int ambient) {
		mPc = pc;
		state = RUN;
		name = text = null;
		choices = null;
		textChanged = true;
		fast = false;
		stopPc = -1;
		mFastAmb = ambient;
		if (ambient >= 0) mAudio.playAmbient(ambient);
	}

	private void endFast(long now) {
		fast = false;
		mStage.finishAnimations(now);
		if (mFastAmb >= 0) mAudio.playAmbient(mFastAmb);
		else mAudio.stopAmbient();
	}

	int startPc() { return mS.startPc; }

	/** Runs until the script blocks. */
	void run(long now) {
		if (state == SAY) {
			// the line ends when the voice track reaches its end mark; the
			// deadline covers a track that ended early or a stalled player
			int pos = mAudio.voicePos();
			boolean reached = pos >= 0 ? pos >= mLineEnd || mAudio.voiceEnded() : now >= mUntil;
			if (!reached && now < mUntil + 1500) return;
			state = RUN;
			if (text != null) { text = name = null; textChanged = true; }
		} else if (state == PAUSE) {
			if (now < mUntil) return;
			state = RUN;
			if (text != null) { text = name = null; textChanged = true; }
		}
		byte[] c = mS.code;
		while (state == RUN) {
			if (mPc == stopPc && !fast) {
				state = STOPPED;
				break;
			}
			int op = c[mPc++];
			switch (op) {
			case OP_LABEL:
				mHost.autosave(mPc - 1, mAudio.ambient());
				break;
			case OP_SCENE:
				mStage.scene(Pack.u16(c, mPc), c[mPc + 2], Pack.u16(c, mPc + 3), now);
				mPc += 5;
				break;
			case OP_SHOW:
				mStage.show(Pack.u16(c, mPc), Pack.u16(c, mPc + 2), c[mPc + 4], Pack.u16(c, mPc + 5), now);
				mPc += 7;
				break;
			case OP_HIDE:
				mStage.hide(Pack.u16(c, mPc), c[mPc + 2]);
				mPc += 3;
				break;
			case OP_PLAY:
				if (c[mPc] == 0 && fast) mFastAmb = Pack.u16(c, mPc + 1);
				else if (c[mPc] == 0) mAudio.playAmbient(Pack.u16(c, mPc + 1));
				mPc += 5;
				break;
			case OP_STOP:
				if (c[mPc] == 0 && fast) mFastAmb = -1;
				else if (c[mPc] == 0) mAudio.stopAmbient();
				else mAudio.stopMovie();
				mPc += 3;
				break;
			case OP_VOICE:
				if (!fast) mAudio.voiceTrack(Pack.u16(c, mPc));
				mPc += 2;
				break;
			case OP_SAY: {
				if (fast) {
					mPc += 11;
					break;
				}
				int who = c[mPc] & 0xFF, str = Pack.u16(c, mPc + 1);
				int b = Pack.s32(c, mPc + 3), e = Pack.s32(c, mPc + 7);
				mPc += 11;
				String n = mS.chars[who];
				name = "none".equals(n) ? null : n;
				text = mS.strings[str];
				textChanged = true;
				// like the original's seekvoice: only re-seek if the track has
				// drifted from this line's start mark
				int pos = mAudio.voicePos();
				if (pos >= 0 && Math.abs(pos - b) > 600) mAudio.seekVoice(b);
				mLineEnd = e;
				mLineBegin = b;
				mLineLen = Math.max(e - b, 500);
				mSayAt = now;
				mUntil = now + Math.max(e - b, 500);
				state = SAY;
				break;
			}
			case OP_MENU: {
				if (fast) endFast(now);
				menuPc = mPc - 1;
				int n = c[mPc++];
				choices = new String[n];
				mTargets = new int[n];
				for (int i = 0; i < n; i++, mPc += 6) {
					choices[i] = mS.strings[Pack.u16(c, mPc)];
					mTargets[i] = Pack.s32(c, mPc + 2);
				}
				sel = 0;
				textChanged = true;
				state = MENU;
				break;
			}
			case OP_JUMP:
				mPc = Pack.s32(c, mPc);
				break;
			case OP_ENDING:
				mHost.ending(c[mPc++] & 0xFF);
				break;
			case OP_TEXT:
				mPc += 4;   // in-story texts are drawn by the scene's own art
				break;
			case OP_PAUSE:
				if (fast) {
					mPc += 4;
					break;
				}
				mUntil = now + Pack.s32(c, mPc);
				mPc += 4;
				state = PAUSE;
				break;
			case OP_RETURN:
			default:
				if (fast) endFast(now);
				state = ENDED;
				break;
			}
		}
	}

	/** Advance: skips the current line or pause, or picks the menu choice. */
	void click() {
		if (state == SAY) {
			// skip the rest of the line: jump the track to its end mark, which
			// is where the next line starts, so the room tone carries on
			mAudio.seekVoice(mLineEnd);
			mUntil = 0;
			mLineEnd = 0;
		} else if (state == PAUSE) {
			mUntil = 0;
			// a skipped "show movie with Pause(n)" must not keep playing its audio
			mAudio.stopMovie();
		} else if (state == MENU) {
			mPc = mTargets[sel];
			choices = null;
			textChanged = true;
			state = RUN;
		}
	}

	/** How far through the current line playback is, 0..1000 (by voice position, or time). */
	int lineProgress(long now) {
		if (state != SAY) return 0;
		int pos = mAudio.voicePos();
		long at = pos >= 0 ? pos - mLineBegin : now - mSayAt;
		if (at <= 0) return 0;
		return at >= mLineLen ? 1000 : (int) (at * 1000 / mLineLen);
	}

	/** Moves playback to a point within the current line (0..1000), e.g. its next page. */
	void seekLine(int per1000, long now) {
		if (state != SAY) return;
		int off = (int) ((long) mLineLen * per1000 / 1000);
		mAudio.seekVoice(mLineBegin + off);
		mSayAt = now - off;
	}

	/** Picks menu option i (auto mode). */
	void choose(int i) {
		if (state != MENU) return;
		sel = i;
		click();
	}

	void move(int d) {
		if (state != MENU) return;
		sel = (sel + d + choices.length) % choices.length;
		textChanged = true;
	}
}
