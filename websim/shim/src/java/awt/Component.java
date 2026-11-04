package java.awt;

import java.awt.image.BufferedImage;
import java.awt.image.ImageObserver;

/** Headless component: just geometry and visibility. The browser page draws the device. */
public class Component implements ImageObserver, java.io.Serializable {
    int x, y, width, height;
    boolean visible;
    Container parent;

    public void setLocation(int x, int y) { this.x = x; this.y = y; }
    public void setLocation(Point p) { setLocation(p.x, p.y); }
    public Point getLocation() { return new Point(x, y); }
    public Point getLocationOnScreen() { return getLocation(); }
    public void setSize(int w, int h) { width = w; height = h; }
    public void resize(int w, int h) { setSize(w, h); }
    public Dimension getSize() { return new Dimension(width, height); }
    public int getWidth() { return width; }
    public int getHeight() { return height; }
    public int getX() { return x; }
    public int getY() { return y; }
    public Rectangle getBounds() { return new Rectangle(x, y, width, height); }
    public void setBounds(int x, int y, int w, int h) { setLocation(x, y); setSize(w, h); }
    public void setVisible(boolean b) { visible = b; }
    public boolean isVisible() { return visible; }
    public boolean isShowing() { return visible; }
    public void show() { setVisible(true); }
    public void hide() { setVisible(false); }
    public void repaint() {}
    public void repaint(long tm) {}
    public void repaint(int x, int y, int w, int h) {}
    public void paint(Graphics g) {}
    public void update(Graphics g) { paint(g); }
    public void invalidate() {}
    public void validate() {}
    public void requestFocus() {}
    public void setBackground(Color c) {}
    public void setForeground(Color c) {}
    public void setFont(Font f) {}
    public void setEnabled(boolean b) {}
    public Container getParent() { return parent; }
    public Toolkit getToolkit() { return Toolkit.getDefaultToolkit(); }
    public FontMetrics getFontMetrics(Font f) { return Toolkit.getDefaultToolkit().getFontMetrics(f); }
    public Image createImage(int w, int h) { return new BufferedImage(Math.max(1, w), Math.max(1, h), BufferedImage.TYPE_INT_RGB); }
    public Graphics getGraphics() { return new BufferedImage(Math.max(1, width), Math.max(1, height), BufferedImage.TYPE_INT_RGB).createGraphics(); }
    public void addMouseListener(java.awt.event.MouseListener l) {}
    public void addMouseWheelListener(java.awt.event.MouseWheelListener l) {}
    public void addKeyListener(java.util.EventListener l) {}
    protected final void enableEvents(long mask) {}
    public boolean action(Event e, Object what) { return false; }
}
