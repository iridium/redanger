package com.redanger.classof09;

import danger.app.Bundle;
import danger.app.DataStore;
import danger.app.Registrar;
import java.io.ByteArrayOutputStream;
import java.lang.reflect.Method;

/**
 * Ending texts in the phone's real Text Messaging app.
 *
 * The SMS app keeps every message as a record in its DataStore
 * "ds:sms_messages_v3" (apps/sms/SMSFolderManager). A text is written there
 * the way SMSMessageContainer.writeToStream lays it out, then the SMS app is
 * told to reload.
 *
 * Opening another app's store (DataStore.findDataStore(name, true)) is off
 * limits to third-party bundles, so it's reached by reflection. A real phone
 * refuses that, and available() is false: the game then keeps the texts in
 * its own Messages screen. The simulator allows it.
 */
final class Sms {
	/** The SMS app's store, by full name (owning bundle class + ':' + name). */
	private static final String STORE = "apps.sms.SMS:ds:sms_messages_v3";
	private static final int INBOX = 5000;                   // SMSResources.kInboxFolderID
	private static final int REC_UNREAD = 0, REC_READ = 1;   // SMSParserConstants.kRec*
	private static final int FLAG_UNREAD = 2;                // what SMSMessageFolder counts as unread
	private static final int STORAGE_DEVICE = 1;

	private static DataStore sStore;
	private static boolean sRefused;

	private Sms() {}

	/** True if texts can go into the real inbox. */
	static boolean available() {
		return store() != null;
	}

	private static DataStore store() {
		if (sStore != null || sRefused) return sStore;
		try {
			Method m = DataStore.class.getMethod("findDataStore", new Class[] { String.class, boolean.class });
			sStore = (DataStore) m.invoke(null, new Object[] { STORE, Boolean.TRUE });
		} catch (Throwable t) {
			sRefused = true;   // the phone's sandbox; no point asking again
			System.out.println("classof09: no SMS inbox: " + t);
		}
		return sStore;
	}

	/**
	 * Puts a text from `from` in the inbox, unless the same one is already
	 * there and not `again`. Returns null on success (or if it was there),
	 * else what went wrong.
	 */
	static String deliver(String from, String text, boolean read, boolean again) {
		DataStore ds = store();
		if (ds == null) return "no SMS inbox";
		try {
			int[] date = new int[1];
			byte[] rec = record(from, text, read, date);
			if (!again && has(ds, rec, date[0])) return null;
			ds.addRecord(rec);
			refresh();
			return null;
		} catch (Throwable t) {
			return t.toString();
		}
	}

	static boolean open() {
		try {
			Bundle b = Bundle.findByName("apps.sms");
			if (b == null) {
				System.out.println("classof09: no Messaging app");
				return false;
			}
			Registrar.bringToForeground(b);   // what the launcher does; starts it if need be
			return true;
		} catch (Throwable t) {
			System.out.println("classof09: can't open Messaging: " + t);
			return false;
		}
	}

	/** An existing record equal to rec, ignoring its read state (status, flags) and the date at dateAt. */
	private static boolean has(DataStore ds, byte[] rec, int dateAt) {
		for (int i = ds.getRecordCount() - 1; i >= 0; i--) {
			byte[] r = ds.getRecordData(i);
			if (r == null || r.length != rec.length) continue;
			int k = 0;
			while (k < r.length && (r[k] == rec[k] || (k >= 4 && k < 12) || (k >= dateAt && k < dateAt + 4))) k++;
			if (k == r.length) return true;
		}
		return false;
	}

	/**
	 * The SMS app reads its store at boot. Ask it to reload: its folder
	 * manager (SMS.getFolderManager(), public static) re-reads every record on
	 * kCmd_RestoreMessagesFromDatastore (32826).
	 */
	private static void refresh() {
		try {
			Class c = Class.forName("apps.sms.SMS");
			Object fm = c.getMethod("getFolderManager", new Class[0]).invoke(null, new Object[0]);
			if (fm != null) ((danger.app.Listener) fm).sendEvent(32826);
		} catch (Throwable t) {
			System.out.println("classof09: SMS reload failed: " + t);
		}
	}

	/**
	 * SMSMessageContainer.writeToStream, one part, in danger.io.ObjectOutputStream's
	 * encoding. The header is what SMSPDUParser makes of an ordinary
	 * SMS-DELIVER (the same for every sender): only the address differs, as
	 * digits (type 0) for a phone number or as given (type 5, alphanumeric).
	 */
	private static byte[] record(String from, String text, boolean read, int[] dateAt) {
		String digits = digits(from);
		ByteArrayOutputStream o = new ByteArrayOutputStream();
		writeInt(o, 0);                                       // SMS-DELIVER
		writeInt(o, read ? REC_READ : REC_UNREAD);
		writeInt(o, read ? 0 : FLAG_UNREAD);                  // flags
		writeInt(o, INBOX);
		writeString(o, "");                                   // no SMSC
		writeInt(o, -1);                                      // SMSC address type
		writeInt(o, 4);                                       // message class: none
		writeInt(o, 0);                                       // protocol id
		writeInt(o, 0);                                       // data coding group
		writeInt(o, -1);                                      // concatenation ref
		writeInt(o, 0);                                       // not concatenated
		writeInt(o, 0);                                       // no reply path
		writeInt(o, -1);                                      // delivery report status
		writeInt(o, -1);                                      // destination port
		writeInt(o, -1);                                      // message waiting
		dateAt[0] = o.size();
		writeInt(o, (int) (System.currentTimeMillis() / 1000));
		writeString(o, digits != null ? digits : from);
		writeInt(o, digits != null ? 0 : 5);                 // address type
		writeInt(o, 0);                                       // user data: text
		writeInt(o, 1);                                       // message size (parts)
		writeInt(o, 1);                                       // part count
		// SMSMessagePart: slot (0: not on the SIM), sequence, text, storage
		writeInt(o, 0);
		writeInt(o, 0);
		writeString(o, text);
		writeInt(o, STORAGE_DEVICE);
		return o.toByteArray();
	}

	/** "703-359-4747" -> "7033594747"; null if it isn't a phone number. */
	private static String digits(String s) {
		StringBuffer d = new StringBuffer();
		for (int i = 0; i < s.length(); i++) {
			char c = s.charAt(i);
			if (c >= '0' && c <= '9') d.append(c);
			else if (c != '-' && c != ' ') return null;
		}
		return d.length() > 0 ? d.toString() : null;
	}

	private static void writeInt(ByteArrayOutputStream o, int v) {
		o.write(v >> 24);
		o.write(v >> 16);
		o.write(v >> 8);
		o.write(v);
	}

	/** Length (7 bits a byte, high group first, 0x80 on all but the last), then the bytes. */
	private static void writeString(ByteArrayOutputStream o, String s) {
		byte[] b = s.getBytes();
		int n = b.length;
		if (n >= 1 << 21) o.write(((n >> 21) & 0x7F) | 0x80);
		if (n >= 1 << 14) o.write(((n >> 14) & 0x7F) | 0x80);
		if (n >= 1 << 7) o.write(((n >> 7) & 0x7F) | 0x80);
		o.write(n & 0x7F);
		o.write(b, 0, n);
	}
}
