package java.awt.image;

import java.awt.Image;

public class PixelGrabber {
    private final Image img;
    private final int x, y, w, h, off, scan;
    private final int[] pix;

    public PixelGrabber(Image img, int x, int y, int w, int h, int[] pix, int off, int scan) {
        this.img = img; this.x = x; this.y = y; this.w = w; this.h = h; this.pix = pix; this.off = off; this.scan = scan;
    }

    public boolean grabPixels() throws InterruptedException {
        if (img instanceof BufferedImage) ((BufferedImage) img).getRGB(x, y, w, h, pix, off, scan);
        return true;
    }
}
