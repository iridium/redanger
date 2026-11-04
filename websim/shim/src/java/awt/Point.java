package java.awt;

public class Point implements java.io.Serializable {
    public int x, y;
    public Point() {}
    public Point(int x, int y) { this.x = x; this.y = y; }
    public Point(Point p) { this(p.x, p.y); }
    public double getX() { return x; }
    public double getY() { return y; }
    public void translate(int dx, int dy) { x += dx; y += dy; }
    public void setLocation(int x, int y) { this.x = x; this.y = y; }
    public boolean equals(Object o) { return o instanceof Point && ((Point) o).x == x && ((Point) o).y == y; }
    public String toString() { return "java.awt.Point[x=" + x + ",y=" + y + "]"; }
}
