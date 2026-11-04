package com.redanger.classof09;

import danger.app.SettingsDB;

/**
 * Auto mode: follows the route worked out ahead of time (tools/auto_plan.py,
 * the script's "auto" table). Each segment restarts at a scene, fast-forwards
 * to the menu it left off at, takes its listed choices, and ends at a stop
 * point or an ending. Flavor choices play back to back up to where they
 * rejoin, branch choices are each played to their end, and nothing that has
 * already played is played again. Progress is saved per segment, so a run
 * survives the phone locking or the game being closed.
 */
final class AutoPlay {
	private final SettingsDB mDb;
	private final Script mS;
	/** current segment, -1 when no run is in progress */
	private int mSeg;
	/** next choice within the segment */
	private int mChoice;
	/** the route is done and the finale (bonus scene) is playing */
	boolean finale;

	AutoPlay(SettingsDB db, Script s) {
		mDb = db;
		mS = s;
		mSeg = num("auto.seg", -1);
		if (mSeg >= mS.autoStart.length) mSeg = -1;   // a new script with a shorter route
		finale = num("auto.finale", 0) != 0;
	}

	/** True if a run is in progress. */
	boolean started() {
		return mSeg >= 0 || finale;
	}

	/** Starts a run if none is in progress; the segment to (re)play is seg(). */
	void begin() {
		if (mSeg < 0) mSeg = 0;
		mChoice = 0;
		save();
	}

	int seg() { return mSeg; }

	/** The planned option at the next menu, or -1 if the segment has none left. */
	int choose(int n) {
		byte[] c = mS.autoChoices[mSeg];
		if (mChoice >= c.length || c[mChoice] >= n) return -1;
		return c[mChoice++];
	}

	/** On to the next segment; false when the route is done. */
	boolean next() {
		mChoice = 0;
		if (mSeg + 1 >= mS.autoStart.length) return false;
		mSeg++;
		save();
		return true;
	}

	/** Segments done / segments in the route. */
	String progress() {
		return Math.max(mSeg, 0) + "/" + mS.autoStart.length;
	}

	void startFinale() {
		finale = true;
		save();
	}

	void reset() {
		mSeg = -1;
		mChoice = 0;
		finale = false;
		save();
	}

	private void save() {
		mDb.setIntValue("auto.seg", mSeg);
		mDb.setIntValue("auto.finale", finale ? 1 : 0);
	}

	private int num(String k, int def) {
		try {
			return mDb.find(k) < 0 ? def : mDb.getIntValue(k);
		} catch (Throwable t) {
			return def;
		}
	}
}
