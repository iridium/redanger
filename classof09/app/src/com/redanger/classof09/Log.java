package com.redanger.classof09;

import danger.storage.StorageManager;
import java.io.FileOutputStream;
import java.io.PrintStream;

/**
 * Errors go to the console and to classof09/.log.txt on the card (a dot file,
 * so the media player's library scan ignores it). htconsole needs USB, and USB
 * lends the card to the computer, so the console isn't available while the
 * game can actually run; the file is read afterwards.
 */
final class Log {
	private Log() {}

	static String root() {
		try {
			String[] p = StorageManager.getRemovablePaths();
			if (p != null && p.length > 0) return p[0];
		} catch (Throwable t) {}
		return "/removable0";
	}

	static void e(String msg, Throwable t) {
		String line = msg + (t == null ? "" : ": " + t);
		System.out.println("classof09: " + line);
		if (t != null) t.printStackTrace();
		try {
			PrintStream ps = new PrintStream(new FileOutputStream(root() + "/classof09/.log.txt", true));
			ps.println(new java.util.Date() + " " + line);
			if (t != null) t.printStackTrace(ps);
			ps.close();
		} catch (Throwable x) {}
	}
}
