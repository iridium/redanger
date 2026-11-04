package javax.imageio;

import java.awt.image.BufferedImage;
import java.awt.image.RenderedImage;
import java.io.File;
import java.io.IOException;
import java.io.InputStream;

/** Image decoding is delegated to the host (the browser's decoders). */
public final class ImageIO {
    public static void setUseCache(boolean b) {}

    public static BufferedImage read(InputStream in) throws IOException {
        java.io.ByteArrayOutputStream out = new java.io.ByteArrayOutputStream();
        byte[] buf = new byte[4096];
        for (int n; (n = in.read(buf)) > 0; ) out.write(buf, 0, n);
        int[] wh = new int[2];
        int[] px = websim.WebHost.decodeImage(out.toByteArray(), wh);
        if (px == null) return null;
        BufferedImage img = new BufferedImage(wh[0], wh[1], BufferedImage.TYPE_INT_ARGB);
        System.arraycopy(px, 0, img.pixels, 0, px.length);
        return img;
    }

    public static boolean write(RenderedImage img, String format, File f) throws IOException { return false; }
    public static java.util.Iterator getImageReadersByFormatName(String n) {
        java.util.Vector v = new java.util.Vector();
        if ("jpeg".equalsIgnoreCase(n) || "jpg".equalsIgnoreCase(n)) v.addElement(new ImageReader());   // sizes only
        return v.iterator();
    }
    public static java.util.Iterator getImageWritersByFormatName(String n) { return new java.util.Vector().iterator(); }
    public static javax.imageio.stream.ImageInputStream createImageInputStream(Object o) {
        return o instanceof InputStream ? new javax.imageio.stream.ImageInputStream((InputStream) o) : null;
    }
    public static Object createImageOutputStream(Object o) { return null; }
}
