package java.awt;

public class Rectangle implements java.io.Serializable {
    public int x, y, width, height;
    public Rectangle() {}
    public Rectangle(int x, int y, int w, int h) { this.x = x; this.y = y; width = w; height = h; }
    public Rectangle(int w, int h) { this(0, 0, w, h); }
    public void translate(int dx, int dy) { x += dx; y += dy; }
    public boolean contains(int px, int py) { return px >= x && py >= y && px < x + width && py < y + height; }
    public String toString() { return "java.awt.Rectangle[x=" + x + ",y=" + y + ",width=" + width + ",height=" + height + "]"; }
}
