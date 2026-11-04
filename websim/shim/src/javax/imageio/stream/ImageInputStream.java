package javax.imageio.stream;

import java.io.InputStream;

/** What ImageIO.createImageInputStream hands to an ImageReader: the stream it wraps. */
public class ImageInputStream {
    public final InputStream in;

    public ImageInputStream(InputStream in) {
        this.in = in;
    }
}
