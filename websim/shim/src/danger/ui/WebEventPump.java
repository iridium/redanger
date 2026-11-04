package danger.ui;

import danger.app.Event;
import danger.app.Listener;
import danger.system.Hardware;
import java.awt.event.KeyEvent;

/** Feeds the page's key events into the simulator's keyboard, like the AWT event thread did. */
public final class WebEventPump implements Runnable {
    /** Event ids for keys sent as Hiptop key codes, for keys a PC keyboard doesn't have. */
    static final int RAW_DOWN = 501, RAW_UP = 502;

    public void run() {
        while (true) {
            int[] e = websim.WebHost.nextKeyEvent();
            if (e[0] == RAW_DOWN || e[0] == RAW_UP) {
                // as AWTScreen.dispatchEvent: -73 key down, -74 key up
                EventManager.processEvent(new Event((Listener) null, e[0] == RAW_DOWN ? -73 : -74, e[1], 0, 0,
                    Hardware.getSystemTime(), (Object) null));
                continue;
            }
            SimKeyboard kb = keyboard();
            if (kb == null) continue;
            kb.processEvent(new KeyEvent(this, e[0], System.currentTimeMillis(), e[3], e[1], (char) e[2]));
        }
    }

    /** AWTScreen.sAWTScreen.mKeyboard, read by the host since both fields are private. */
    static native SimKeyboard keyboard();
}
