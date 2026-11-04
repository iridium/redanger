package websim;

/** Calls into the JS host page. */
public final class WebHost {
    /** Blocks until the page has a key event: {id, keyCode, keyChar, modifiers}. */
    public static native int[] nextKeyEvent();

    /** Decodes PNG/GIF/JPEG with the browser; returns ARGB pixels and fills wh, or null. */
    public static native int[] decodeImage(byte[] data, int[] wh);

    /** True if a DataStore changed since the last call (DataStore.updateLastModified ran). */
    public static native boolean storesChanged();
}
