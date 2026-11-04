package websim;

/** Pixel loops; the JS host overrides these with native versions for speed. */
public final class Raster {
    public static void fill(int[] px, int stride, int x0, int y0, int x1, int y1, int argb, int rule, float alpha) {
        int a = (int) (((argb >>> 24) & 255) * alpha);
        for (int y = y0; y < y1; y++) {
            int p = y * stride;
            for (int x = x0; x < x1; x++) px[p + x] = blend(px[p + x], argb, a, rule);
        }
    }

    public static void blit(int[] src, int sstride, boolean srcAlpha, int sx, int sy, int sw, int sh,
                            int[] dst, int dstride, int dx, int dy, int dw, int dh,
                            int cx0, int cy0, int cx1, int cy1, int rule, float alpha) {
        int y0 = Math.max(cy0, dy), y1 = Math.min(cy1, dy + dh);
        int x0 = Math.max(cx0, dx), x1 = Math.min(cx1, dx + dw);
        for (int y = y0; y < y1; y++) {
            int srow = (sy + (y - dy) * sh / dh) * sstride;
            for (int x = x0; x < x1; x++) {
                int s = src[srow + sx + (x - dx) * sw / dw];
                if (!srcAlpha) s |= 0xff000000;
                int a = (int) (((s >>> 24) & 255) * alpha);
                dst[y * dstride + x] = blend(dst[y * dstride + x], s, a, rule);
            }
        }
    }

    static int blend(int d, int s, int a, int rule) {
        if (rule == 2) return s; // SRC
        if (rule == 1) return 0; // CLEAR
        if (a >= 255) return s | 0xff000000;
        if (a <= 0) return d;
        int ia = 255 - a;
        int r = (((s >> 16) & 255) * a + ((d >> 16) & 255) * ia) / 255;
        int g = (((s >> 8) & 255) * a + ((d >> 8) & 255) * ia) / 255;
        int b = ((s & 255) * a + (d & 255) * ia) / 255;
        return 0xff000000 | (r << 16) | (g << 8) | b;
    }
}
