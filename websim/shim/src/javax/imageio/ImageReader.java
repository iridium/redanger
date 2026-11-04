package javax.imageio;

import java.io.IOException;
import java.io.InputStream;

/**
 * Just enough of a JPEG reader for SimJPEGCodecImpl.getDimensions: width and height from the
 * frame header (SOFn), without decoding. Decoding goes through ImageIO.read.
 */
public class ImageReader {
    private InputStream mIn;
    private int mWidth = -1, mHeight = -1;

    public void setInput(Object input) {
        if (input instanceof javax.imageio.stream.ImageInputStream) input = ((javax.imageio.stream.ImageInputStream) input).in;
        mIn = (InputStream) input;
        mWidth = mHeight = -1;
    }

    public int getWidth(int image) throws IOException {
        if (mWidth < 0) readHeader();
        return mWidth;
    }

    public int getHeight(int image) throws IOException {
        if (mHeight < 0) readHeader();
        return mHeight;
    }

    public void dispose() {
        mIn = null;
    }

    private void readHeader() throws IOException {
        InputStream in = mIn;
        if (in == null || in.read() != 0xFF || in.read() != 0xD8) throw new IOException("not a JPEG");
        while (true) {
            int b = in.read();
            if (b < 0) break;
            if (b != 0xFF) continue;
            int m;
            do m = in.read(); while (m == 0xFF);   // fill bytes
            if (m < 0) break;
            if (m == 0xD8 || m == 0x01 || (m >= 0xD0 && m <= 0xD7)) continue;   // no length
            if (m == 0xD9 || m == 0xDA) break;   // end, or entropy-coded data before any frame
            int len = u16(in);
            // SOF0..SOF15, except DHT (C4), JPG (C8) and DAC (CC)
            if (m >= 0xC0 && m <= 0xCF && m != 0xC4 && m != 0xC8 && m != 0xCC) {
                in.read();   // sample precision
                mHeight = u16(in);
                mWidth = u16(in);
                return;
            }
            skip(in, len - 2);
        }
        throw new IOException("no JPEG frame header");
    }

    private static int u16(InputStream in) throws IOException {
        int a = in.read(), b = in.read();
        if ((a | b) < 0) throw new IOException("truncated JPEG");
        return (a << 8) | b;
    }

    private static void skip(InputStream in, int n) throws IOException {
        while (n > 0) {
            long k = in.skip(n);
            if (k <= 0) {
                if (in.read() < 0) throw new IOException("truncated JPEG");
                k = 1;
            }
            n -= k;
        }
    }
}
