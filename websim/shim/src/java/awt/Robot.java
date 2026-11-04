package java.awt;

import java.awt.image.BufferedImage;

/** The simulator's scripting robot; key injection is not supported in the browser. */
public class Robot {
    public Robot() throws AWTException {}
    public void keyPress(int keycode) {}
    public void keyRelease(int keycode) {}
    public void delay(int ms) {}
    public BufferedImage createScreenCapture(Rectangle r) { return new BufferedImage(r.width, r.height, BufferedImage.TYPE_INT_RGB); }
}
