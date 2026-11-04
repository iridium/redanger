package com.redanger.classof09;

import danger.audio.AudioManager;
import danger.audio.Playable;
import danger.audio.Sound;
import danger.audio.Stream;
import java.io.IOException;

/**
 * All audio is MP3 played by the device's native decoder (danger.audio.Stream).
 * Each sound is also a plain file on the card (snd/.<n>.mp3), which the native
 * player streams by name, so even a whole scene's voice track never sits in the
 * Java heap. If that fails, the copy inside the pack is loaded into memory.
 */
final class Audio {
	private final Pack mPack;
	private Playable mVoice, mAmbient, mMovie, mSfx;
	private int mAmbientSnd = -1;

	Audio(Pack pack) {
		mPack = pack;
	}

	int ambient() { return mAmbientSnd; }

	// The scene's voice track plays straight through, like the original: it
	// carries room tone under the dialogue, so it must not stop between lines.
	// Playback position comes from the player; a wall-clock estimate covers
	// players that don't report it.
	private long mVoiceAt;
	private int mVoiceBase;

	void voiceTrack(int snd) {
		stopVoice();
		mVoice = start(snd, false);
		mVoiceBase = 0;
		mVoiceAt = System.currentTimeMillis();
	}

	/** Position in the current voice track (ms), or -1 if none. */
	int voicePos() {
		if (mVoice == null) return -1;
		int est = mVoiceBase + (int) (System.currentTimeMillis() - mVoiceAt);
		try {
			int p = mVoice.getMicrosecondPosition() / 1000;
			if (p > 0) return p;
		} catch (Throwable t) {}
		return est;
	}

	void seekVoice(int ms) {
		if (mVoice == null) return;
		try {
			mVoice.setMicrosecondPosition(ms * 1000);
		} catch (Throwable t) {}
		mVoiceBase = ms;
		mVoiceAt = System.currentTimeMillis();
	}

	boolean voiceEnded() {
		return mVoice == null || mVoice.isDone();
	}

	void stopVoice() {
		mVoice = kill(mVoice);
	}

	void playAmbient(int snd) {
		if (snd == mAmbientSnd && mAmbient != null) return;
		stopAmbient();
		mAmbientSnd = snd;
		mAmbient = start(snd, true);
	}

	void stopAmbient() {
		mAmbient = kill(mAmbient);
		mAmbientSnd = -1;
	}

	void playMovie(int snd) {
		stopMovie();
		mMovie = start(snd, false);
	}

	void stopMovie() {
		mMovie = kill(mMovie);
	}

	/** One-shot effect on its own channel (e.g. the new-text alert). */
	void sfx(int snd) {
		mSfx = kill(mSfx);
		mSfx = start(snd, false);
	}

	void stopAll() {
		mSfx = kill(mSfx);
		stopVoice();
		stopAmbient();
		stopMovie();
	}

	void tick(long now) {}

	private byte[] read(int entry) {
		try {
			return mPack.read(entry);
		} catch (IOException e) {
			return null;
		} catch (OutOfMemoryError e) {
			Log.e("audio: no memory for a " + mPack.length(entry) + " byte clip", e);
			return null;
		}
	}

	/**
	 * Starts sound `snd`: streamed by file name if possible, else from the pack.
	 * Each start tries the best path again, so one failure (e.g. a moment of
	 * memory pressure) never silences the rest of the game.
	 */
	private Playable start(int snd, boolean loop) {
		Playable a = stream(mPack.soundFile(snd), null, loop);
		if (a == null) {
			byte[] b = read(mPack.soundEntry(snd));
			if (b == null) return null;
			a = stream(null, b, loop);
			if (a == null) {
				try {
					Sound sn = Sound.load(b, false);
					if (sn != null && loop) sn.setLoopingEnabled(true);
					a = sn;
				} catch (Throwable t) {
					Log.e("audio: Sound refused clip " + snd, t);
				}
			}
		}
		if (a == null) return null;
		a.startDefault();
		return a;
	}

	/** A native MP3 stream from a file name, or from bytes; null if refused. */
	private static Playable stream(String file, byte[] mp3, boolean loop) {
		Stream s = null;
		try {
			s = AudioManager.createStream(false);
			if (s != null && (file != null ? s.loadFromFileName(file) : s.loadFromFile(mp3))) {
				if (loop) s.setLoopingEnabled(true);
				return s;
			}
		} catch (Throwable t) {
			Log.e("audio: Stream failed for " + (file != null ? file : mp3.length + " bytes"), t);
		}
		if (s != null) s.destroy();
		return null;
	}

	private static Playable kill(Playable a) {
		if (a != null) a.destroy();
		return null;
	}
}
