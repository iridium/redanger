package java.awt.image;

import java.awt.Graphics;
import java.awt.Graphics2D;
import java.awt.Image;

public class BufferedImage extends Image implements RenderedImage {
    public static final int TYPE_CUSTOM = 0, TYPE_INT_RGB = 1, TYPE_INT_ARGB = 2, TYPE_INT_ARGB_PRE = 3,
        TYPE_INT_BGR = 4, TYPE_3BYTE_BGR = 5, TYPE_4BYTE_ABGR = 6, TYPE_BYTE_GRAY = 10, TYPE_BYTE_INDEXED = 13;

    public final int[] pixels; // ARGB, row-major
    final int width, height, type;

    public BufferedImage(int w, int h, int type) {
        width = w;
        height = h;
        this.type = type;
        pixels = new int[w * h];
        if (!hasAlpha()) java.util.Arrays.fill(pixels, 0xff000000);
    }

    public boolean hasAlpha() { return type == TYPE_INT_ARGB || type == TYPE_INT_ARGB_PRE || type == TYPE_4BYTE_ABGR; }
    public int getType() { return type; }
    public int getWidth() { return width; }
    public int getHeight() { return height; }
    public int getWidth(ImageObserver o) { return width; }
    public int getHeight(ImageObserver o) { return height; }
    public Graphics getGraphics() { return createGraphics(); }
    public Graphics2D createGraphics() { return new Graphics2D(this); }

    public int getRGB(int x, int y) { return pixels[y * width + x]; }
    public void setRGB(int x, int y, int argb) { pixels[y * width + x] = hasAlpha() ? argb : argb | 0xff000000; }

    public int[] getRGB(int x, int y, int w, int h, int[] out, int off, int scan) {
        if (out == null) out = new int[off + h * scan];
        for (int j = 0; j < h; j++) System.arraycopy(pixels, (y + j) * width + x, out, off + j * scan, w);
        return out;
    }

    public void setRGB(int x, int y, int w, int h, int[] in, int off, int scan) {
        for (int j = 0; j < h; j++) System.arraycopy(in, off + j * scan, pixels, (y + j) * width + x, w);
    }
}
