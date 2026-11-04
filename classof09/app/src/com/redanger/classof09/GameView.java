package com.redanger.classof09;

import danger.app.Application;
import danger.app.Event;
import danger.app.EventType;
import danger.app.SettingsDB;
import danger.ui.Font;
import danger.ui.MarqueeAlert;
import danger.ui.NotificationManager;
import danger.ui.Pen;
import danger.ui.Screen;
import danger.ui.ShortBitmap;
import danger.ui.ScreenWindow;
import java.util.Vector;

/** Full-screen window: splash movie, title menu, and the game itself. */
final class GameView extends ScreenWindow implements Stage.Listener, Vm.Host {
	private static final int SPLASH = 0, TITLE = 1, GAME = 2, ERROR = 3, INBOX = 4, READ = 5, ENDINGS = 6;
	private static final int BOX_TOP = 104;
	private static final int TEXT = 0xFFFFFF, NAME = 0xFFB0D0, SEL = 0xFFE070, DIM = 0xA0A0A0;

	private final int mW, mH;
	private final short[] mFb;
	private ShortBitmap mBmp;
	private final Font mFont, mBold, mSmall;
	private final SettingsDB mDb = new SettingsDB("classof09", false);

	private Pack mPack;
	private Script mScript;
	private Stage mStage;
	private Audio mAudio;
	private Vm mVm;
	private AutoPlay mAuto;
	/** auto mode is driving the game; mAutoResume restarts it after a suspend */
	private boolean mAutoOn, mAutoResume;
	private long mAutoMenuAt;
	private int mAutoPick;
	private int mMode = ERROR, mTitleSel, mInboxSel, mInboxTop, mReadScroll, mEndSel, mEndTop;
	/** Playing an ending picked from Endings: no autosave, and its message always arrives. */
	private boolean mEndingRun;
	/** Ending reached this playthrough; notifies when the game returns to the title. */
	private int mNewEnding = -1;
	private String mError;
	private int mMenuMusic, mTextAlert, mRollover, mPress, mChoice, mPhoneSlide, mPhoneButton, mPhoneClose;
	private Sprite mMovie;
	private boolean mRepaint;
	/** Rows hidden under the system title bar when the window isn't full screen. */
	private int mOff;
	/** Drop out of full screen until then so a notification marquee can scroll. */
	private long mMarqueeUntil;
	private String mTitle = "";
	private long mLastAwake;

	GameView() {
		mW = Screen.getWidth();
		mH = Screen.getHeight();
		setFullScreen(true);
		Img.sSwapRB = Screen.getColorSpace() == 9;
		mFb = new short[mW * mH];
		mBmp = new ShortBitmap(mFb, mW, mH, Screen.getColorSpace(), -1, 0);
		mFont = Font.findSystemFont();
		mBold = Font.findBoldSystemFont();
		Font small = Font.findFont("Bort9");
		mSmall = small != null ? small : mFont;
		open();
	}

	private String packPath() {
		return Log.root() + "/classof09/classof09.c9p";
	}

	private void open() {
		String path = packPath();
		if (!new java.io.File(Pack.onCard(path)).exists()) {
			mError = "Memory card is unavailable. If USB is plugged in, unplug it.\n\n"
				+ "(Needs " + path + ")";
			mMode = ERROR;
			return;
		}
		try {
			mPack = new Pack(path);
			mScript = new Script(mPack.read(0));
		} catch (Throwable t) {
			Log.e("can't open " + path, t);
			mPack = null;
			mError = "Can't read " + path + ": " + t;
			mMode = ERROR;
			return;
		}
		mStage = new Stage(mPack, mScript.atl, this);
		mAudio = new Audio(mPack);
		mVm = new Vm(mScript, mStage, mAudio, this);
		mAuto = new AutoPlay(mDb, mScript);
		mMenuMusic = mScript.find(mScript.sounds, "audio/menu_piano.mp3");
		mTextAlert = mScript.find(mScript.sounds, "audio/NewTextAlert.mp3");
		mRollover = mScript.find(mScript.sounds, "audio/MainMenuRollover.mp3");
		mPress = mScript.find(mScript.sounds, "audio/MainMenuPress.mp3");
		mChoice = mScript.find(mScript.sounds, "audio/PhoneSelect.mp3");
		mPhoneSlide = mScript.find(mScript.sounds, "audio/Phone_slide.mp3");
		mPhoneButton = mScript.find(mScript.sounds, "audio/Phone_button.mp3");
		mPhoneClose = mScript.find(mScript.sounds, "audio/Phone_close.mp3");
		// simulator testing: -Dclassof09.endings=32767 marks every ending found
		String e = System.getProperty("classof09.endings");
		if (e != null) mDb.setIntValue("endings", Integer.parseInt(e));
		syncTexts();
		mMode = SPLASH;
		mStage.show(0, mScript.find(mScript.images, "@splash"), 0, 0xFFFF, now());
	}

	private static long now() {
		return System.currentTimeMillis();
	}

	// The original's main_logo_transform: centred at (0.7, 0.3) of the screen,
	// 1.2x a 580x395 box, easing up to 1.3x in 0.35 s and back down over 1.5 s.
	private static final int LOGO_TAG = 0x7FFF;
	private static final int LOGO_Z0 = 580, LOGO_Z1 = 628;   // zoom (per mille of the packed art) for 1.2x and 1.3x
	private long mLogoT0;

	/** Sets the logo's size and position for time t; true if it changed. */
	private boolean updateLogo(long t) {
		Sprite s = mStage.sprite(LOGO_TAG, 1);
		if (s == null || s.img == null) return false;
		int ph = (int) ((t - mLogoT0) % 1850);
		int z;
		if (ph < 350) {
			int f = ph * 1000 / 350;
			z = LOGO_Z0 + (LOGO_Z1 - LOGO_Z0) * (1000000 - (1000 - f) * (1000 - f)) / 1000000;
		} else {
			z = LOGO_Z1 - (LOGO_Z1 - LOGO_Z0) * (ph - 350) / 1500;
		}
		int dw = s.img.w * z / 1000, dh = s.img.h * z / 1000;
		int cx = Stage.W * 7 / 10, cy = Stage.H * 3 / 10;
		int xa = Stage.W > dw ? (cx - dw / 2) * 1000 / (Stage.W - dw) : 500;
		int ya = Stage.H > dh ? (cy - dh / 2) * 1000 / (Stage.H - dh) : 500;
		int[] p = s.prop;
		if (p[Sprite.ZOOM] == z && p[Sprite.XALIGN] == xa && p[Sprite.YALIGN] == ya) return false;
		p[Sprite.ZOOM] = z;
		p[Sprite.XALIGN] = xa;
		p[Sprite.YALIGN] = ya;
		return true;
	}

	/** Auto mode, between paths: sit on the title (and its notification) until then. */
	private long mAutoTitleUntil;
	/**
	 * Auto mode, after a new ending: the ending to open in the reader once the
	 * title's notification has shown, then whether the reader is open (it
	 * scrolls through long texts on its own) and when it next moves.
	 */
	private int mAutoReadEnding = -1;
	private boolean mAutoReading, mAutoReadEnd;
	private long mAutoReadAt;
	private static final int READ_HOLD = 3000, READ_LINE = 1200;

	private void title() {
		syncTexts();
		mEndingRun = false;
		mAutoOn = false;
		mAutoTitleUntil = 0;
		mAutoMenuAt = 0;
		mMode = TITLE;
		mAudio.stopAll();
		mStage.clearAll();
		mStage.show(0, mScript.find(mScript.images, "@menu_bg"), 0, 0xFFFF, now());
		int logo = mScript.find(mScript.images, "@logo");
		if (logo >= 0) mStage.show(LOGO_TAG, logo, 1, 0xFFFF, now());
		mLogoT0 = now();
		updateLogo(mLogoT0);
		if (mMenuMusic >= 0) mAudio.playAmbient(mMenuMusic);
		mTitleSel = hasSave() ? 1 : 0;
		mRepaint = true;
		// simulator testing: -Dclassof09.auto=1 starts auto mode from the title
		if (System.getProperty("classof09.auto") != null && !mAutoStartedByProp) {
			mAutoStartedByProp = true;
			startAuto();
			return;
		}
		if (mNewEnding >= 0) {
			notify("New message from " + mScript.endSender[mNewEnding]);
			mNewEnding = -1;
		}
	}

	private void play(boolean resume) {
		mAudio.stopAll();
		mStage.clearAll();
		mMode = GAME;
		if (resume && hasSave()) mVm.start(get("pc", 0), get("amb", -1));
		else mVm.start(debugPc(), -1);
	}

	/** Simulator testing: -Dclassof09.pc=N starts New Game at that bytecode offset. */
	private int debugPc() {
		String p = System.getProperty("classof09.pc");
		return p == null ? mVm.startPc() : Integer.parseInt(p);
	}

	private boolean hasSave() { return get("pc", -1) >= 0; }

	private int get(String k, int def) {
		try {
			return mDb.find(k) < 0 ? def : mDb.getIntValue(k);
		} catch (Throwable t) {
			return def;
		}
	}

	public void autosave(int pc, int ambient) {
		if (mEndingRun) return;   // replaying an ending doesn't touch the player's save
		if (mAutoOn) return;   // auto mode doesn't touch the player's own save
		mDb.setIntValue("pc", pc);
		mDb.setIntValue("amb", ambient);
	}

	public void ending(int id) {
		int i = endingIndex(id);
		if (i < 0) return;
		int seen = get("endings", 0);
		if ((seen & (1 << i)) == 0 || mEndingRun) {
			mNewEnding = i;
			mDb.setIntValue("read", get("read", 0) & ~(1 << i));
			if (Sms.available()) text(i, false, mEndingRun);
		}
		mDb.setIntValue("endings", seen | (1 << i));
	}

	/**
	 * An ending's text into the real Text Messaging inbox. Unless `again` (a
	 * replay from Endings, which sends it anew), one that's already there isn't
	 * added twice.
	 */
	private boolean text(int i, boolean read, boolean again) {
		String err = Sms.deliver(mScript.endSender[i], mScript.endBody[i], read, again);
		if (err != null) System.out.println("classof09: text not delivered: " + err);
		return err == null;
	}

	/**
	 * The phone keeps texts only while it runs (the simulator saves its stores
	 * at power off at best), so on launch put back any found ending's text
	 * that's missing. Also tried from the title, in case Messaging hadn't set
	 * up its store yet when the game opened.
	 */
	private boolean mSynced;

	private void syncTexts() {
		if (mSynced || !Sms.available()) return;
		mSynced = true;
		int f = found(), read = get("read", 0), n = 0;
		for (int i = 0; i < mScript.endId.length; i++) {
			if ((f & (1 << i)) != 0 && text(i, (read & (1 << i)) != 0, false)) n++;
		}
		if (n > 0) System.out.println("classof09: " + n + " ending texts in Messaging");
	}


	private void notify(String msg) {
		if (mTextAlert >= 0) mAudio.sfx(mTextAlert);
		mMarqueeUntil = now() + 7000;
		try {
			Application app = Application.getCurrentApp();
			NotificationManager.marqueeAlertNotify(
				new MarqueeAlert(msg, app == null ? null : app.getBundle().getSmallIcon(), 2));
		} catch (Throwable t) {
			System.out.println("classof09: notify failed: " + t);
		}
	}

	private int endingIndex(int id) {
		for (int i = 0; i < mScript.endId.length; i++) if (mScript.endId[i] == id) return i;
		return -1;
	}

	private int found() { return get("endings", 0); }
	private int unread() { return found() & ~get("read", 0); }

	private int endingsSeen() {
		int m = found(), n = 0;
		for (; m != 0; m &= m - 1) n++;
		return n;
	}

	public void movieStarted(Sprite s, int img) {
		mMovie = s;
		int snd = mScript.find(mScript.sounds, mScript.images[img]);
		if (snd >= 0) mAudio.playMovie(snd);
	}

	/**
	 * The title, inbox and reader are normal titled windows, so the system bar
	 * (app name, clock, notification marquee) shows like in any native app. The
	 * game is full screen except while a text notification scrolls by.
	 */
	private void layout(long t) {
		boolean titled = mMode == INBOX || mMode == READ || mMode == ENDINGS
			|| ((mMode == GAME || mMode == TITLE) && t < mMarqueeUntil);
		if (titled == isFullScreen()) {
			setFullScreen(!titled);
			mRepaint = true;
		}
		String title = "Class of '09";
		if (mMode == INBOX) title = "Messages " + endingsSeen() + "/" + mScript.endId.length
			+ "  " + (get("bonus", 0) != 0 ? 1 : 0) + "/1";
		else if (mMode == ENDINGS) title = "Endings";
		else if (mMode == READ) title = "From: " + mScript.endSender[inbox()[mInboxSel]];
		if (!title.equals(mTitle)) {
			setTitle(title);
			mTitle = title;
		}
		mOff = mH - getHeight();
	}

	/**
	 * No auto-lock while a scene plays or Auto Play runs. The system's lock app
	 * asks the active window before engaging the keyguard (Screen.keyGuardIsBlocked)
	 * and, if blocked, just restarts its timer. Registration screens use the same
	 * override. On the title and Messages screens the phone locks as usual.
	 */
	public boolean blocksKeyGuard() {
		return mMode == GAME || mMode == SPLASH || mAutoOn || mAutoTitleUntil != 0 || mAutoReading;
	}

	/** Voiced scenes run for minutes without a keypress: keep the screen lit (see Awake). */
	private void stayAwake(long t) {
		if (t - mLastAwake < 3000) return;
		mLastAwake = t;
		Awake.poke();
	}

	private void restart(int pc, int ambient) {
		mAudio.stopAll();
		mStage.clearAll();
		mMode = GAME;
		mAutoMenuAt = 0;
		mVm.start(pc, ambient);
	}

	private void startAuto() {
		mAutoOn = true;
		if (mAuto.finale && mScript.bonusPc != 0xFFFFFFFF) {
			restart(mScript.bonusPc, -1);
		} else {
			mAuto.begin();
			playSegment();
		}
		mAutoOn = true;
	}

	/** Auto mode: (re)play the route's current segment from its start. */
	private void playSegment() {
		int i = mAuto.seg();
		restart(mScript.autoStart[i], mScript.autoAmb[i]);
		// a later segment fast-forwards to the menu it left off at
		mVm.fast = mScript.autoFast[i];
		mVm.stopPc = mScript.autoStop[i];
		mAutoOn = true;
	}

	/** Auto mode: open the new ending's text in the reader, like a player would. */
	private void autoOpenReader(int ending, long t) {
		int[] list = inbox();
		mInboxSel = 0;
		for (int i = 0; i < list.length; i++) if (list[i] == ending) mInboxSel = i;
		mInboxTop = Math.max(0, mInboxSel - 3);
		sfx(mPhoneSlide);
		mDb.setIntValue("read", get("read", 0) | (1 << ending));
		mReadScroll = 0;
		mMode = READ;
		mAutoReading = true;
		mAutoReadEnd = false;
		mAutoReadAt = t + READ_HOLD;
		mRepaint = true;
	}

	/** Auto mode, in the reader: scroll a line at a time, hold at the end, then leave. */
	private void autoReadStep(long t) {
		if (mReadScroll < readMaxScroll()) {
			mReadScroll++;
			mAutoReadAt = t + READ_LINE;
		} else if (!mAutoReadEnd) {
			mAutoReadEnd = true;
			mAutoReadAt = t + READ_HOLD;
			return;
		} else {
			mAutoReading = false;
			sfx(mPhoneClose);
			mMode = TITLE;
			mAutoTitleUntil = t + 1500;
		}
		mRepaint = true;
	}

	/** Lines the reader can scroll by for the open text. */
	private int readMaxScroll() {
		int[] list = inbox();
		if (mInboxSel >= list.length || list[mInboxSel] == BONUS) return 0;
		Vector lines = wrap(mFont, mScript.endBody[list[mInboxSel]], mW - 8);
		int rows = (getHeight() - 2) / lineH(mFont);
		return Math.max(0, lines.size() - rows);
	}

	/** A segment ended (at its stop point or an ending): on to the next, or finish. */
	private void autoNext() {
		if (mAuto.next()) {
			playSegment();
		} else {
			// every option taken, so every ending found: play the hidden outro
			mAuto.startFinale();
			if (mScript.bonusPc != 0xFFFFFFFF) {
				restart(mScript.bonusPc, -1);
				mAutoOn = true;
			} else {
				mAuto.reset();
				title();
			}
		}
	}

	/** At a menu: show the pick for a moment, then take it. */
	private void autoMenu(long t) {
		if (mAutoMenuAt == 0) {
			mAutoPick = mAuto.choose(mVm.choices.length);
			if (mAutoPick < 0) {
				autoNext();
				return;
			}
			mAutoMenuAt = t;
			mVm.sel = mAutoPick;
			mVm.textChanged = true;
		} else if (t - mAutoMenuAt > 1500) {
			mAutoMenuAt = 0;
			sfx(mChoice);
			mVm.choose(mAutoPick);
		}
	}

	private long mLastCardCheck;

	/** The card vanished (USB mass storage) or failed: stop and wait for it. */
	void cardLost() {
		if (mAudio != null) mAudio.stopAll();
		if (mPack != null) mPack.close();
		mPack = null;
		mMovie = null;
		mError = "Memory card is unavailable. If USB is plugged in, unplug it.";
		mMode = ERROR;
		mRepaint = false;
	}

	/** Every 2 s: notice the card going away, or coming back. */
	private void checkCard(long t) {
		if (t - mLastCardCheck < 2000) return;
		mLastCardCheck = t;
		boolean present = new java.io.File(Pack.onCard(packPath())).exists();
		if (mPack != null && !present) {
			cardLost();
		} else if (mPack == null && present) {
			open();
			mRepaint = true;
		}
	}

	void tick() {
		stayAwake(now());
		checkCard(now());
		if (mMode != ERROR) layout(now());
		if (mMode == ERROR) {
			if (!mRepaint) {
				if (!isFullScreen()) setFullScreen(true);
				mRepaint = true;
				invalidate();
			}
			return;
		}
		long t = now();
		mAudio.tick(t);
		if (mMode == GAME) {
			if (mAutoOn && mVm.state == Vm.MENU) autoMenu(t);
			mVm.run(t);
			if (mVm.state == Vm.STOPPED) {
				// the segment reached its stop point: straight on to the next
				autoNext();
				return;
			}
			if (mVm.state == Vm.ENDED) {
				boolean auto = mAutoOn && !mAuto.finale;
				int news = mNewEnding;
				title();
				if (auto) {
					// like a real playthrough: back on the title (with the ending's
					// text notification), then read the new text, then on to the
					// next branch
					mAutoReadEnding = Sms.available() ? -1 : news;
					mAutoTitleUntil = t + (news >= 0 ? 4000 : 3000);
				} else if (mAuto.finale) {
					mAuto.reset();
				}
				return;
			}
		}
		if (mMode == TITLE && mAutoTitleUntil != 0 && t >= mAutoTitleUntil) {
			mAutoTitleUntil = 0;
			if (mAutoReadEnding >= 0) {
				autoOpenReader(mAutoReadEnding, t);
				mAutoReadEnding = -1;
				return;
			}
			mAutoOn = true;
			autoNext();
			return;
		}
		if (mMode == READ && mAutoReading && t >= mAutoReadAt) autoReadStep(t);
		if (mMovie != null && mMovie.video != null && mMovie.video.done() && mMode == SPLASH) {
			mMovie = null;
			title();
		}
		// the movie left the stage (scene change, hide, skip): silence its audio
		if (mMovie != null && mStage.movie() != mMovie) {
			mAudio.stopMovie();
			mMovie = null;
		}
		boolean changed = mStage.tick(t);
		if (mMode == TITLE && updateLogo(t)) changed = true;
		if (mVm.textChanged) {
			mVm.textChanged = false;
			changed = true;
		}
		if (mMode == GAME && mVm.state == Vm.SAY && pageChanged(t)) changed = true;
		if (changed || mRepaint) {
			mRepaint = false;
			compose();
			invalidate();
		}
	}

	private void compose() {
		mStage.composite(mFb, mW);
		for (int i = Stage.H * mW; i < mFb.length; i++) mFb[i] = 0;
		if (mMode == GAME && mVm.text != null) darken(BOX_TOP, mH);
		if (mMode == GAME && mVm.state == Vm.MENU) darken(0, mH);
		if (mMode == TITLE) darken(titleTop() - 4, mH);
		// The simulator caches a bitmap as an AWT image on first draw. A fresh
		// wrapper around the same array (no copy) sidesteps that cache.
		mBmp = new ShortBitmap(mFb, mW, mH, Screen.getColorSpace(), -1, 0);
	}

	private void darken(int y0, int y1) {
		for (int i = y0 * mW; i < y1 * mW; i++) mFb[i] = (short) ((mFb[i] >> 1) & 0x7BEF);
	}

	public void paint(Pen p) {
		p.drawBitmap(0, -mOff, mBmp);
		if (mMode == ERROR) {
			p.setColor(0);
			p.fillRect(0, 0, mW, mH);
			drawWrapped(p, mFont, "Class of '09: " + mError, 4, 4, mW - 8, 10, TEXT);
		} else if (mMode == TITLE) {
			String[] items = items();
			// centred in the darkened box (titleTop() - 4 down to the bottom); 1px
			// higher because the system font's ascent leaves 2 blank rows above capitals
			int box = titleTop() - 4;
			int y = box + (mH - box - items.length * lineH(mFont)) / 2 - 1 - mOff;
			for (int i = 0; i < items.length; i++) {
				p.setFont(i == mTitleSel ? mBold : mFont);
				p.setColor(i == mTitleSel ? SEL : TEXT);
				p.drawText(10, y + mFont.getAscent(), (i == mTitleSel ? "> " : "  ") + items[i]);
				y += lineH(mFont);
			}
		} else if (mMode == GAME) {
			if (mVm.state == Vm.MENU) paintMenu(p);
			else if (mVm.text != null) paintSay(p);
		} else if (mMode == INBOX) {
			paintInbox(p);
		} else if (mMode == READ) {
			paintRead(p);
		} else if (mMode == ENDINGS) {
			paintEndings(p);
		}
	}

	// The original draws these on a PDA graphic; here the Sidekick's own screen
	// is the phone, styled after the built-in Text Messaging app.

	private static final int ROW_SEL = 0x101848, INK = 0x000000, GREY = 0x606060;

	/** Inbox row for the bonus video message (every ending found). */
	private static final int BONUS = -1;

	private boolean allFound() {
		return endingsSeen() == mScript.endId.length && mScript.bonusPc != 0xFFFFFFFF;
	}

	/** Inbox rows: the bonus (if unlocked), then found endings in tracker order. */
	private int[] inbox() {
		int f = found(), n = allFound() ? 1 : 0;
		for (int i = 0; i < mScript.endId.length; i++) if ((f & (1 << i)) != 0) n++;
		int[] out = new int[n];
		int k = 0;
		if (allFound()) out[k++] = BONUS;
		for (int i = 0; i < mScript.endId.length; i++) if ((f & (1 << i)) != 0) out[k++] = i;
		return out;
	}

	private void paintInbox(Pen p) {
		int h = getHeight();
		p.setColor(0xFFFFFF);
		p.fillRect(0, 0, mW, h);
		int y = 0;
		int[] list = inbox();
		int rowH = lineH(mFont) + 3, rows = h / rowH;
		if (mInboxSel < mInboxTop) mInboxTop = mInboxSel;
		if (mInboxSel >= mInboxTop + rows) mInboxTop = mInboxSel - rows + 1;
		int unread = unread();
		for (int r = mInboxTop; r < list.length && r < mInboxTop + rows; r++, y += rowH) {
			int e = list[r];
			boolean sel = r == mInboxSel;
			boolean isNew = e == BONUS ? get("bonus", 0) == 0 : (unread & (1 << e)) != 0;
			String from = e == BONUS ? "UNKNOWN" : mScript.endSender[e];
			String preview = e == BONUS ? "VIDEO MESSAGE" : mScript.endPreview[e];
			if (sel) {
				p.setColor(ROW_SEL);
				p.fillRect(0, y, mW, y + rowH);
			}
			Font f = isNew ? mBold : mFont;
			p.setFont(f);
			p.setColor(sel ? 0xFFFFFF : INK);
			p.drawText(4, y + 1 + f.getAscent(), fit(f, from, 84));
			p.setFont(mFont);
			p.setColor(sel ? 0xFFFFFF : GREY);
			p.drawText(92, y + 1 + mFont.getAscent(), fit(mFont, preview, mW - 96));
		}
	}

	private void paintRead(Pen p) {
		int[] list = inbox();
		int e = list[mInboxSel];
		int h = getHeight();
		p.setColor(0xFFFFFF);
		p.fillRect(0, 0, mW, h);
		int y = 2;
		Vector lines = wrap(mFont, mScript.endBody[e], mW - 8);
		int lh = lineH(mFont), rows = (h - y) / lh;
		if (mReadScroll > Math.max(0, lines.size() - rows)) mReadScroll = Math.max(0, lines.size() - rows);
		p.setFont(mFont);
		p.setColor(INK);
		for (int i = mReadScroll; i < lines.size() && i < mReadScroll + rows; i++, y += lh) {
			p.drawText(4, y + mFont.getAscent(), (String) lines.elementAt(i));
		}
	}

	private static String fit(Font f, String s, int w) {
		if (f.getWidth(s) <= w) return s;
		int n = s.length();
		while (n > 0 && f.getWidth(s.substring(0, n) + "...") > w) n--;
		return s.substring(0, n) + "...";
	}

	private String[] items() {
		return new String[] { "New Game", hasSave() ? "Continue" : "Continue (no save)",
			mAuto.started() ? "Auto Play (resume " + mAuto.progress() + ")" : "Auto Play",
			"Messages " + endingsSeen() + "/" + mScript.endId.length + (!Sms.available() && unread() != 0 ? "  NEW" : ""),
			"Endings" };
	}

	/** Top of the title menu: the dialogue box's top, raised if the items don't fit under it. */
	private int titleTop() {
		return Math.min(BOX_TOP, mH - 2 - 5 * lineH(mFont));
	}

	// Every ending, to watch again: picking one plays its scene and sends its
	// message, as if reached in a playthrough.

	/** Names the fans use (Class of '09 wiki), by ending id; matched by each ending's message. */
	private static final int[] END_NAME_ID = { 39, 63, 74, 91, 103, 105, 121, 126, 127, 144, 145, 153, 161, 169, 173 };
	private static final String[] END_NAME = { "Necrophilia", "School Shooting", "Moving Again", "Chlorine Bomb",
		"FBI", "School Arson", "Drug Rehab", "YouTube Famous", "Valedictorian", "L.A. Dream", "Homeless",
		"State-wide Expulsion", "Graduation", "Psych Ward", "Suicide" };

	private String endingName(int e) {
		for (int i = 0; i < END_NAME_ID.length; i++) if (END_NAME_ID[i] == mScript.endId[e]) return END_NAME[i];
		return "Ending";
	}

	/** Per ending (tracker order): the start of the scene that reaches it, or -1. */
	private int[] mEndingPc;

	private int endingPc(int e) {
		if (mEndingPc == null) mEndingPc = scanEndings();
		return mEndingPc[e];
	}

	/** Walks the bytecode once: each ending's scene is the LABEL before its first ENDING. */
	private int[] scanEndings() {
		int[] out = new int[mScript.endId.length];
		for (int i = 0; i < out.length; i++) out[i] = -1;
		byte[] c = mScript.code;
		int pc = 0, label = -1;
		while (pc < c.length) {
			int op = c[pc];
			switch (op) {
			case 13: label = pc; pc += 1; break;                // LABEL
			case 1: pc += 6; break;                             // SCENE
			case 2: pc += 8; break;                             // SHOW
			case 3: case 5: pc += 4; break;                     // HIDE, STOP
			case 4: pc += 6; break;                             // PLAY
			case 6: pc += 3; break;                             // VOICE
			case 7: pc += 12; break;                            // SAY
			case 8: pc += 2 + 6 * c[pc + 1]; break;             // MENU
			case 9: case 11: case 14: pc += 5; break;           // JUMP, PAUSE, TEXT
			case 10: {                                          // ENDING
				int i = endingIndex(c[pc + 1] & 0xFF);
				if (i >= 0 && out[i] < 0) out[i] = label;
				pc += 2;
				break;
			}
			default: pc += 1; break;                            // RETURN
			}
		}
		return out;
	}

	/** The hidden outro, unlocked by finding every ending. */
	private void playBonus() {
		mDb.setIntValue("bonus", 1);
		mAudio.stopAll();
		mStage.clearAll();
		mMode = GAME;
		mVm.start(mScript.bonusPc, -1);
	}

	/** Rows in the Endings list: every ending, then the bonus video once all are found. */
	private int endingRows() {
		return mScript.endId.length + (allFound() ? 1 : 0);
	}

	private void playEnding(int e) {
		if (e == mScript.endId.length) {
			playBonus();
			return;
		}
		int pc = endingPc(e);
		if (pc < 0) return;
		sfx(mPress);
		mAudio.stopAll();
		mStage.clearAll();
		mMode = GAME;
		mEndingRun = true;
		mVm.start(pc, -1);
	}

	private void paintEndings(Pen p) {
		int h = getHeight();
		p.setColor(0xFFFFFF);
		p.fillRect(0, 0, mW, h);
		int n = endingRows(), rowH = lineH(mFont) + 3, rows = h / rowH, y = 0;
		if (mEndSel < mEndTop) mEndTop = mEndSel;
		if (mEndSel >= mEndTop + rows) mEndTop = mEndSel - rows + 1;
		for (int r = mEndTop; r < n && r < mEndTop + rows; r++, y += rowH) {
			boolean sel = r == mEndSel;
			if (sel) {
				p.setColor(ROW_SEL);
				p.fillRect(0, y, mW, y + rowH);
			}
			p.setFont(mFont);
			p.setColor(sel ? 0xFFFFFF : INK);
			if (r == mScript.endId.length) {
				p.drawText(22, y + 1 + mFont.getAscent(), "Bonus video");
				continue;
			}
			p.drawText(4, y + 1 + mFont.getAscent(), (r + 1) + ".");
			p.drawText(22, y + 1 + mFont.getAscent(), endingName(r));
		}
	}

	// A line that doesn't fit the box even in the small font is split into
	// pages. Pages follow the voice (each gets a share of the line's time in
	// proportion to its length), and a click turns to the next page.
	private String mPagedFor;
	private Font mPageFont;
	private Vector mPageLines;
	private int mPageRows, mPageShown;
	private int[] mPageStart;   // per page: where it starts in the line, 0..1000

	private void layoutPages() {
		if (mVm.text == mPagedFor) return;
		mPagedFor = mVm.text;
		mPageShown = 0;
		if (mPagedFor == null) return;
		int top = BOX_TOP + 2 + (mVm.name != null ? lineH(mBold) : 0);
		Font f = mFont;
		Vector lines = wrap(f, mPagedFor, mW - 8);
		if (lines.size() > (mH - top) / lineH(f)) {
			f = mSmall;
			lines = wrap(f, mPagedFor, mW - 8);
		}
		int rows = Math.max(1, (mH - top) / lineH(f));
		int pages = (lines.size() + rows - 1) / rows;
		mPageFont = f;
		mPageLines = lines;
		mPageRows = rows;
		mPageStart = new int[Math.max(pages, 1)];
		int total = 0;
		for (int i = 0; i < lines.size(); i++) total += ((String) lines.elementAt(i)).length() + 1;
		int seen = 0;
		for (int pg = 0, i = 0; pg < pages; pg++) {
			mPageStart[pg] = (int) ((long) seen * 1000 / Math.max(total, 1));
			for (int k = 0; k < rows && i < lines.size(); k++, i++) seen += ((String) lines.elementAt(i)).length() + 1;
		}
	}

	/** The page due now; true if it differs from the one on screen. */
	private boolean pageChanged(long t) {
		layoutPages();
		if (mPagedFor == null || mPageStart.length < 2) return false;
		int prog = mVm.lineProgress(t), pg = 0;
		while (pg + 1 < mPageStart.length && mPageStart[pg + 1] <= prog) pg++;
		if (pg < mPageShown) return false;   // never page backwards (e.g. a stalled player)
		boolean changed = pg != mPageShown;
		mPageShown = pg;
		return changed;
	}

	private void paintSay(Pen p) {
		int y = BOX_TOP + 2 - mOff;
		if (mVm.name != null) {
			p.setFont(mBold);
			p.setColor(NAME);
			p.drawText(4, y + mBold.getAscent(), mVm.name);
			y += lineH(mBold);
		}
		layoutPages();
		p.setFont(mPageFont);
		p.setColor(TEXT);
		int first = mPageShown * mPageRows;
		for (int i = first; i < mPageLines.size() && i < first + mPageRows; i++) {
			p.drawText(4, y + mPageFont.getAscent(), (String) mPageLines.elementAt(i));
			y += lineH(mPageFont);
		}
		if (mPageShown + 1 < mPageStart.length) {
			p.setFont(mSmall);
			p.setColor(NAME);
			String more = (mPageShown + 1) + "/" + mPageStart.length;
			p.drawText(mW - 4 - mSmall.getWidth(more), mH - mOff - 2 - mSmall.getDescent(), more);
		}
	}

	private void paintMenu(Pen p) {
		String[] c = mVm.choices;
		int total = 0;
		for (int i = 0; i < c.length; i++) total += wrap(mBold, c[i], mW - 24).size() * lineH(mBold) + 6;
		int y = Math.max(2, (mH - mOff - total) / 2);
		for (int i = 0; i < c.length; i++) {
			Vector lines = wrap(mBold, c[i], mW - 24);
			int h = lines.size() * lineH(mBold) + 4;
			boolean sel = i == mVm.sel;
			p.setColor(sel ? 0x503040 : 0x202020);
			p.fillRect(8, y, mW - 8, y + h);
			drawWrapped(p, mBold, c[i], 12, y + 2, mW - 24, 9, sel ? SEL : DIM);
			y += h + 2;
		}
	}

	private static int lineH(Font f) {
		return f.getAscent() + f.getDescent();
	}

	private static void drawWrapped(Pen p, Font f, String s, int x, int y, int w, int maxLines, int color) {
		Vector lines = wrap(f, s, w);
		p.setFont(f);
		p.setColor(color);
		for (int i = 0; i < lines.size() && i < maxLines; i++) {
			p.drawText(x, y + f.getAscent(), (String) lines.elementAt(i));
			y += lineH(f);
		}
	}

	/** Greedy word wrap; honours explicit newlines. */
	private static Vector wrap(Font f, String s, int w) {
		Vector out = new Vector();
		int start = 0;
		while (start <= s.length()) {
			int nl = s.indexOf('\n', start);
			String para = nl < 0 ? s.substring(start) : s.substring(start, nl);
			StringBuffer line = new StringBuffer();
			int i = 0;
			while (i < para.length()) {
				int sp = para.indexOf(' ', i);
				String word = sp < 0 ? para.substring(i) : para.substring(i, sp);
				String trial = line.length() == 0 ? word : line + " " + word;
				if (line.length() > 0 && f.getWidth(trial) > w) {
					out.addElement(line.toString());
					line = new StringBuffer(word);
				} else {
					line = new StringBuffer(trial);
				}
				i = sp < 0 ? para.length() : sp + 1;
			}
			out.addElement(line.toString());
			if (nl < 0) break;
			start = nl + 1;
		}
		return out;
	}

	private void sfx(int snd) {
		if (snd >= 0 && mAudio != null) mAudio.sfx(snd);
	}

	private boolean stopAutoReader() {
		if (!mAutoReading && mAutoReadEnding < 0) return false;
		mAutoReading = false;
		mAutoReadEnding = -1;
		mAutoTitleUntil = 0;
		mRepaint = true;
		return true;
	}

	private void click() {
		if (stopAutoReader()) return;
		if (mAutoTitleUntil != 0) { mAutoTitleUntil = 0; mRepaint = true; return; }
		if (mMode == SPLASH) {
			title();
		} else if (mMode == TITLE) {
			if (mTitleSel == 0) { play(false); sfx(mPress); }
			else if (mTitleSel == 1 && hasSave()) { play(true); sfx(mPress); }
			else if (mTitleSel == 2) { startAuto(); sfx(mPress); }
			else if (mTitleSel == 4) {
				sfx(mPhoneSlide);
				mMode = ENDINGS;
				mRepaint = true;
			}
			else if (mTitleSel == 3 && Sms.available()) {
				sfx(mPhoneSlide);
				Sms.open();
			}
			else if (mTitleSel == 3 && found() != 0) {
				sfx(mPhoneSlide);
				mMode = INBOX;
				mInboxSel = mInboxTop = 0;
				mRepaint = true;
			}
		} else if (mMode == INBOX && inbox()[mInboxSel] == BONUS) {
			sfx(mPhoneButton);
			playBonus();
		} else if (mMode == INBOX) {
			int[] list = inbox();
			if (list.length > 0 && list[mInboxSel] != BONUS) {
				sfx(mPhoneButton);
				mDb.setIntValue("read", get("read", 0) | (1 << list[mInboxSel]));
				mReadScroll = 0;
				mMode = READ;
				mRepaint = true;
			}
		} else if (mMode == READ) {
			back();
		} else if (mMode == ENDINGS) {
			playEnding(mEndSel);
		} else if (mMode == GAME) {
			if (mVm.state == Vm.MENU) {
				if (mAutoOn) return;   // auto mode picks
				mVm.click();
				sfx(mChoice);   // the original's choice screen plays PhoneSelect
			} else if (mVm.state == Vm.SAY && mPageStart != null && mVm.text == mPagedFor
					&& mPageShown + 1 < mPageStart.length) {
				// turn the page: move the voice to where the next page starts
				mVm.seekLine(mPageStart[++mPageShown], now());
				mRepaint = true;
			} else {
				mVm.click();
			}
		}
	}

	private void move(int d) {
		if (stopAutoReader()) return;
		if (mAutoTitleUntil != 0) { mAutoTitleUntil = 0; mRepaint = true; return; }
		if (mMode == TITLE) {
			sfx(mRollover);
			mTitleSel = (mTitleSel + d + 5) % 5;
			mRepaint = true;
		} else if (mMode == GAME) {
			if (mVm.state == Vm.MENU && !mAutoOn) mVm.move(d);
		} else if (mMode == INBOX) {
			int n = inbox().length;
			if (n > 0) mInboxSel = (mInboxSel + d + n) % n;
			mRepaint = true;
		} else if (mMode == ENDINGS) {
			int n = endingRows();
			mEndSel = (mEndSel + d + n) % n;
			mRepaint = true;
		} else if (mMode == READ) {
			mReadScroll = Math.max(0, mReadScroll + d);
			mRepaint = true;
		}
	}

	/** Back: reader -> inbox -> title. Returns false when there is nowhere to go. */
	private boolean back() {
		stopAutoReader();
		if (mMode == READ) { sfx(mPhoneButton); mMode = INBOX; }
		else if (mMode == INBOX || mMode == ENDINGS) { sfx(mPhoneClose); mMode = TITLE; }
		else if (mMode == GAME) { title(); return true; }
		else return false;
		mRepaint = true;
		return true;
	}

	public boolean eventWidgetDown(int w, Event e) {
		switch (w) {
		case EventType.DEVICE_WHEEL_BUTTON: click(); return true;
		case EventType.DEVICE_ARROW_UP: move(-1); return true;
		case EventType.DEVICE_ARROW_DOWN:
		case EventType.DEVICE_WHEEL: move(1); return true;
		case EventType.DEVICE_BUTTON_BACK:
			if (back()) return true;
			break;
		}
		return super.eventWidgetDown(w, e);
	}

	public boolean eventWidgetUp(int w, Event e) {
		if (w == EventType.DEVICE_WHEEL) { move(-1); return true; }
		return super.eventWidgetUp(w, e);
	}

	public boolean eventKeyDown(char c, Event e) {
		if (c == '\n' || c == '\r' || c == ' ') { click(); return true; }
		return super.eventKeyDown(c, e);
	}

	/** Back in the foreground: an interrupted auto run picks up where it was. */
	void resumed() {
		if (mAutoResume && mMode != ERROR) {
			mAutoResume = false;
			if (mAutoResumeNext) {
				// it was between paths (on the title): carry on to the next branch
				mAutoResumeNext = false;
				mAutoOn = true;
				autoNext();
			} else {
				startAuto();
			}
		}
	}

	private boolean mAutoResumeNext, mAutoStartedByProp;

	void suspend() {
		mAutoResume = mAutoOn || mAutoTitleUntil != 0 || mAutoReading;
		mAutoResumeNext = mAutoTitleUntil != 0 || mAutoReading;
		mAutoTitleUntil = 0;
		mAutoReading = false;
		mAutoReadEnding = -1;
		Awake.release();
		if (mAudio != null) mAudio.stopAll();
		if (mMode == GAME) title();
	}

	void quit() {
		Awake.release();
		if (mAudio != null) mAudio.stopAll();
		if (mPack != null) mPack.close();
	}
}
