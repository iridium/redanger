package java.awt;

import java.awt.image.BufferedImage;
import java.awt.image.ImageObserver;
import websim.Raster;

/** Software renderer over a BufferedImage's int[] ARGB pixels. */
public class Graphics {
    final BufferedImage target;
    int tx, ty; // translation
    int cx0, cy0, cx1, cy1; // clip, device coords, exclusive max
    Color color = Color.black;
    Font font;
    Composite composite = AlphaComposite.SrcOver;

    Graphics(BufferedImage target) {
        this.target = target;
        cx1 = target.getWidth();
        cy1 = target.getHeight();
    }

    public Graphics create() {
        Graphics2D g = new Graphics2D(target);
        g.tx = tx; g.ty = ty; g.cx0 = cx0; g.cy0 = cy0; g.cx1 = cx1; g.cy1 = cy1;
        g.color = color; g.font = font; g.composite = composite;
        return g;
    }

    public Graphics create(int x, int y, int w, int h) {
        Graphics g = create();
        g.translate(x, y);
        g.clipRect(0, 0, w, h);
        return g;
    }

    public void dispose() {}
    public void translate(int x, int y) { tx += x; ty += y; }
    public Color getColor() { return color; }
    public void setColor(Color c) { if (c != null) color = c; }
    public Font getFont() { return font; }
    public void setFont(Font f) { font = f; }
    public void setPaintMode() { composite = AlphaComposite.SrcOver; }
    public void setXORMode(Color c) {}

    public void setClip(int x, int y, int w, int h) {
        cx0 = Math.max(0, x + tx);
        cy0 = Math.max(0, y + ty);
        cx1 = Math.min(target.getWidth(), x + tx + w);
        cy1 = Math.min(target.getHeight(), y + ty + h);
    }

    public void clipRect(int x, int y, int w, int h) {
        cx0 = Math.max(cx0, x + tx);
        cy0 = Math.max(cy0, y + ty);
        cx1 = Math.min(cx1, x + tx + w);
        cy1 = Math.min(cy1, y + ty + h);
    }

    public Rectangle getClipBounds() { return new Rectangle(cx0 - tx, cy0 - ty, cx1 - cx0, cy1 - cy0); }

    int rule() { return composite instanceof AlphaComposite ? ((AlphaComposite) composite).rule : AlphaComposite.SRC_OVER; }
    float extraAlpha() { return composite instanceof AlphaComposite ? ((AlphaComposite) composite).alpha : 1f; }

    public void fillRect(int x, int y, int w, int h) {
        int x0 = Math.max(cx0, x + tx), y0 = Math.max(cy0, y + ty);
        int x1 = Math.min(cx1, x + tx + w), y1 = Math.min(cy1, y + ty + h);
        if (x0 >= x1 || y0 >= y1) return;
        Raster.fill(target.pixels, target.getWidth(), x0, y0, x1, y1, color.getRGB(), rule(), extraAlpha());
    }

    public void clearRect(int x, int y, int w, int h) {
        Composite c = composite;
        Color k = color;
        composite = AlphaComposite.Src;
        color = Color.white;
        fillRect(x, y, w, h);
        composite = c;
        color = k;
    }

    public void fillRoundRect(int x, int y, int w, int h, int aw, int ah) { fillRect(x, y, w, h); }

    public void drawRect(int x, int y, int w, int h) {
        fillRect(x, y, w, 1);
        fillRect(x, y + h, w + 1, 1);
        fillRect(x, y, 1, h);
        fillRect(x + w, y, 1, h);
    }

    public void drawLine(int x0, int y0, int x1, int y1) {
        int dx = Math.abs(x1 - x0), dy = -Math.abs(y1 - y0);
        int sx = x0 < x1 ? 1 : -1, sy = y0 < y1 ? 1 : -1, err = dx + dy;
        while (true) {
            fillRect(x0, y0, 1, 1);
            if (x0 == x1 && y0 == y1) break;
            int e2 = 2 * err;
            if (e2 >= dy) { err += dy; x0 += sx; }
            if (e2 <= dx) { err += dx; y0 += sy; }
        }
    }

    public void drawPolygon(int[] xs, int[] ys, int n) {
        for (int i = 0; i < n; i++) {
            int j = (i + 1) % n;
            drawLine(xs[i], ys[i], xs[j], ys[j]);
        }
    }

    public void drawPolyline(int[] xs, int[] ys, int n) {
        for (int i = 0; i + 1 < n; i++) drawLine(xs[i], ys[i], xs[i + 1], ys[i + 1]);
    }

    /** Even-odd scanline fill, sampling pixel centers. */
    public void fillPolygon(int[] xs, int[] ys, int n) {
        if (n < 3) return;
        int minY = Integer.MAX_VALUE, maxY = Integer.MIN_VALUE;
        for (int i = 0; i < n; i++) { minY = Math.min(minY, ys[i]); maxY = Math.max(maxY, ys[i]); }
        int[] nodes = new int[n];
        for (int y = minY; y < maxY; y++) {
            double cy = y + 0.5;
            int k = 0;
            for (int i = 0, j = n - 1; i < n; j = i++) {
                if ((ys[i] <= cy && ys[j] > cy) || (ys[j] <= cy && ys[i] > cy)) {
                    nodes[k++] = (int) Math.ceil(xs[i] + (cy - ys[i]) * (xs[j] - xs[i]) / (double) (ys[j] - ys[i]) - 0.5);
                }
            }
            java.util.Arrays.sort(nodes, 0, k);
            for (int i = 0; i + 1 < k; i += 2) if (nodes[i + 1] > nodes[i]) fillRect(nodes[i], y, nodes[i + 1] - nodes[i], 1);
        }
    }

    public void drawString(String s, int x, int y) {}
    public void drawChars(char[] data, int off, int len, int x, int y) {}

    public boolean drawImage(Image img, int x, int y, ImageObserver o) {
        if (!(img instanceof BufferedImage)) return true;
        BufferedImage src = (BufferedImage) img;
        return drawImage(img, x, y, x + src.getWidth(), y + src.getHeight(), 0, 0, src.getWidth(), src.getHeight(), o);
    }

    public boolean drawImage(Image img, int x, int y, int w, int h, ImageObserver o) {
        if (!(img instanceof BufferedImage)) return true;
        BufferedImage src = (BufferedImage) img;
        return drawImage(img, x, y, x + w, y + h, 0, 0, src.getWidth(), src.getHeight(), o);
    }

    public boolean drawImage(Image img, int dx1, int dy1, int dx2, int dy2, int sx1, int sy1, int sx2, int sy2, ImageObserver o) {
        if (!(img instanceof BufferedImage)) return true;
        BufferedImage src = (BufferedImage) img;
        int dw = dx2 - dx1, dh = dy2 - dy1, sw = sx2 - sx1, sh = sy2 - sy1;
        if (dw <= 0 || dh <= 0 || sw <= 0 || sh <= 0) return true;
        Raster.blit(src.pixels, src.getWidth(), src.hasAlpha(), sx1, sy1, sw, sh,
            target.pixels, target.getWidth(), dx1 + tx, dy1 + ty, dw, dh, cx0, cy0, cx1, cy1, rule(), extraAlpha());
        return true;
    }
}
