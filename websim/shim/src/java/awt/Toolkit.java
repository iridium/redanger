package java.awt;

import java.awt.datatransfer.Clipboard;

public class Toolkit {
    private static final Toolkit sToolkit = new Toolkit();
    private final Clipboard clipboard = new Clipboard("system");

    public static Toolkit getDefaultToolkit() { return sToolkit; }
    public FontMetrics getFontMetrics(Font f) { return new FontMetrics(f) {}; }
    public Clipboard getSystemClipboard() { return clipboard; }
    public Dimension getScreenSize() { return new Dimension(1024, 768); }
    public void beep() {}
    public void sync() {}
}
