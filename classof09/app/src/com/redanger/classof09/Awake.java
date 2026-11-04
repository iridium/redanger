package com.redanger.classof09;

import danger.app.Bundle;
import danger.app.Event;
import danger.app.Registrar;

/**
 * Keeps the screen from dimming or turning off while the game is in the
 * foreground (voiced scenes run for minutes without a keypress).
 *
 * The system app's EVENT_ACTIVATE_BACKLIGHT (1700) handler is what a keypress
 * triggers: it restarts the backlight inactivity timer and, if the light is
 * off, turns it on with its power-management hold. Registrar.getSysAppBundle()
 * and Bundle.sysSendEvent() are both allowed for apps.
 *
 * Don't touch the backlight directly (Hardware.setBacklight / Nokia setLights):
 * re-lighting it without the system's power hold leaves it stuck dim, and the
 * system then thinks it's on, so keypresses no longer restore it. Reflection
 * into danger.system is refused at runtime on the device.
 */
final class Awake {
	private static final int EVENT_ACTIVATE_BACKLIGHT = 1700;
	private static boolean sLogged;

	private Awake() {}

	/** Call every few seconds while in the foreground. */
	static void poke() {
		try {
			Bundle sys = Registrar.getSysAppBundle();
			if (sys != null) sys.sysSendEvent(new Event(EVENT_ACTIVATE_BACKLIGHT));
			if (!sLogged) { sLogged = true; Log.e("keep-awake: sysapp " + (sys != null ? "ok" : "missing"), null); }
		} catch (Throwable t) {
			if (!sLogged) { sLogged = true; Log.e("keep-awake failed", t); }
		}
	}

	/** Nothing is held between pokes; leaving the game lets the timers run out normally. */
	static void release() {}
}
