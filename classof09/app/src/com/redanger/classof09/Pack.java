package com.redanger.classof09;

import java.io.File;
import java.io.IOException;
import java.io.RandomAccessFile;

/** Random access to the pack files on the card (see FORMATS.md). */
final class Pack {
	private final String mDir;
	private final String[] mNames;
	private final RandomAccessFile[] mFiles;
	private final byte[] mFileOf;
	private final int[] mOff, mLen;
	final int nImages, nSounds, nClips;
	private int mDotSounds = -1; // snd/.<n>.mp3 on the card? -1 = not checked yet

	/**
	 * The card copy of a data file: ".name" when present, else "name". The media
	 * player's library scan skips dot files, so hidden names keep it from probing
	 * hundreds of game files (and listing the sound clips as music).
	 */
	static String onCard(String path) {
		int slash = path.lastIndexOf('/');
		String hidden = path.substring(0, slash + 1) + "." + path.substring(slash + 1);
		return new File(hidden).exists() ? hidden : path;
	}

	/**
	 * Opens the index (classof09.c9p). Entries live in it or in the segment
	 * files next to it (images/movies/voice/audio.c9p), opened on first use.
	 */
	Pack(String path) throws IOException {
		mDir = path.substring(0, path.lastIndexOf('/') + 1);
		RandomAccessFile index = new RandomAccessFile(onCard(path), "r");
		byte[] h = new byte[13];
		index.readFully(h);
		if (h[0] != 'C' || h[1] != '9' || h[2] != 'P' || h[3] != '3') {
			index.close();
			throw new IOException("not a C9P3 pack (rebuild it)");
		}
		int nScript = u16(h, 4);
		nImages = u16(h, 6);
		nSounds = u16(h, 8);
		nClips = u16(h, 10);
		int nf = h[12] & 0xFF;
		mNames = new String[nf];
		mFiles = new RandomAccessFile[nf];
		mFiles[0] = index;
		for (int i = 0; i < nf; i++) {
			byte[] n = new byte[index.readUnsignedByte()];
			index.readFully(n);
			mNames[i] = new String(n);
		}
		int n = nScript + nImages + nSounds + nClips;
		byte[] t = new byte[n * 9];
		index.readFully(t);
		mFileOf = new byte[n];
		mOff = new int[n];
		mLen = new int[n];
		for (int i = 0; i < n; i++) {
			mFileOf[i] = t[i * 9];
			mOff[i] = s32(t, i * 9 + 1);
			mLen[i] = s32(t, i * 9 + 5);
		}
	}

	int imageEntry(int img) { return 1 + img; }
	int soundEntry(int snd) { return 1 + nImages + snd; }
	int length(int entry) { return mLen[entry]; }

	/** The card file holding sound `snd` on its own (see Audio). */
	String soundFile(int snd) {
		if (mDotSounds < 0) mDotSounds = new File(mDir + "snd/." + snd + ".mp3").exists() ? 1 : 0;
		return mDir + "snd/" + (mDotSounds == 1 ? "." : "") + snd + ".mp3";
	}

	byte[] read(int entry) throws IOException {
		byte[] b = new byte[mLen[entry]];
		read(entry, 0, b, 0, b.length);
		return b;
	}

	synchronized void read(int entry, int pos, byte[] buf, int off, int n) throws IOException {
		int f = mFileOf[entry];
		if (mFiles[f] == null) mFiles[f] = new RandomAccessFile(onCard(mDir + mNames[f]), "r");
		mFiles[f].seek(mOff[entry] + pos);
		mFiles[f].readFully(buf, off, n);
	}

	void close() {
		for (int i = 0; i < mFiles.length; i++) {
			try { if (mFiles[i] != null) mFiles[i].close(); } catch (IOException e) {}
			mFiles[i] = null;
		}
	}

	static int u16(byte[] b, int o) { return ((b[o] & 0xFF) << 8) | (b[o + 1] & 0xFF); }
	static int s16(byte[] b, int o) { return (short) u16(b, o); }
	static int s32(byte[] b, int o) {
		return ((b[o] & 0xFF) << 24) | ((b[o + 1] & 0xFF) << 16) | ((b[o + 2] & 0xFF) << 8) | (b[o + 3] & 0xFF);
	}

	/** Decodes a C9Z block at b[pos]; returns the raw bytes. */
	static byte[] unz(byte[] b, int pos) {
		byte[] out = new byte[s32(b, pos)];
		Lz4.decode(b, pos + 8, s32(b, pos + 4), out);
		return out;
	}
}
