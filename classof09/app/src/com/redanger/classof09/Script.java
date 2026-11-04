package com.redanger.classof09;

/** The C9S script: string tables, ATL programs and bytecode. */
final class Script {
	final int startPc;
	final String[] strings, chars, images, sounds;
	final byte[][] atl;
	final byte[] code;
	/** The ending tracker's inbox: per ending, sender / preview / full text. */
	final int[] endId;
	final String[] endSender, endPreview, endBody;
	/** All endings unlock this scene (0xFFFFFFFF = none). */
	final int bonusPc;
	/**
	 * Auto Play's route (FORMATS.md "auto"), per segment: the label to start
	 * at and its ambience (-1 = none), whether to fast-forward to the first
	 * menu, where to stop (-1 = at an ending), and the choices to take.
	 */
	final int[] autoStart, autoAmb, autoStop;
	final boolean[] autoFast;
	final byte[][] autoChoices;

	Script(byte[] b) {
		int[] p = { 8 };
		startPc = Pack.s32(b, 4);
		strings = table(b, p);
		chars = table(b, p);
		images = table(b, p);
		sounds = table(b, p);
		int n = Pack.u16(b, p[0]);
		p[0] += 2;
		atl = new byte[n][];
		for (int i = 0; i < n; i++) {
			int len = Pack.u16(b, p[0]);
			atl[i] = new byte[len];
			System.arraycopy(b, p[0] + 2, atl[i], 0, len);
			p[0] += 2 + len;
		}
		int ne = b[p[0]++] & 0xFF;
		endId = new int[ne];
		endSender = new String[ne];
		endPreview = new String[ne];
		endBody = new String[ne];
		for (int i = 0; i < ne; i++, p[0] += 7) {
			endId[i] = b[p[0]] & 0xFF;
			endSender[i] = strings[Pack.u16(b, p[0] + 1)];
			endPreview[i] = strings[Pack.u16(b, p[0] + 3)];
			endBody[i] = strings[Pack.u16(b, p[0] + 5)];
		}
		bonusPc = Pack.s32(b, p[0]);
		p[0] += 6;   // and the tracker's tip text, which isn't shown
		int len = Pack.s32(b, p[0]);
		code = new byte[len];
		System.arraycopy(b, p[0] + 4, code, 0, len);
		p[0] += 4 + len;
		int ns = Pack.u16(b, p[0]);
		p[0] += 2;
		autoStart = new int[ns];
		autoAmb = new int[ns];
		autoStop = new int[ns];
		autoFast = new boolean[ns];
		autoChoices = new byte[ns][];
		for (int i = 0; i < ns; i++) {
			autoStart[i] = Pack.s32(b, p[0]);
			int amb = Pack.u16(b, p[0] + 4);
			autoAmb[i] = amb == 0xFFFF ? -1 : amb;
			autoFast[i] = b[p[0] + 6] != 0;
			autoStop[i] = Pack.s32(b, p[0] + 7);
			autoChoices[i] = new byte[b[p[0] + 11] & 0xFF];
			System.arraycopy(b, p[0] + 12, autoChoices[i], 0, autoChoices[i].length);
			p[0] += 12 + autoChoices[i].length;
		}
	}

	private static String[] table(byte[] b, int[] p) {
		int n = Pack.u16(b, p[0]);
		p[0] += 2;
		String[] t = new String[n];
		for (int i = 0; i < n; i++) {
			int len = Pack.u16(b, p[0]);
			t[i] = utf8(b, p[0] + 2, len);
			p[0] += 2 + len;
		}
		return t;
	}

	static String utf8(byte[] b, int off, int len) {
		StringBuffer s = new StringBuffer(len);
		int end = off + len;
		while (off < end) {
			int c = b[off++] & 0xFF;
			if (c >= 0xE0) {
				c = ((c & 0x0F) << 12) | ((b[off] & 0x3F) << 6) | (b[off + 1] & 0x3F);
				off += 2;
			} else if (c >= 0xC0) {
				c = ((c & 0x1F) << 6) | (b[off++] & 0x3F);
			}
			s.append(fold((char) c));
		}
		return s.toString();
	}

	/** Maps typographic punctuation to what the device fonts have. */
	private static char fold(char c) {
		switch (c) {
		case '\u2018': case '\u2019': return '\'';
		case '\u201C': case '\u201D': return '"';
		case '\u2013': case '\u2014': return '-';
		default: return c;
		}
	}

	int find(String[] t, String name) {
		for (int i = 0; i < t.length; i++) if (t[i].equals(name)) return i;
		return -1;
	}
}
