package java.awt.event;

import java.awt.Point;

public class MouseEvent extends InputEvent {
    int x, y, button, clickCount;

    public MouseEvent(Object source, int id, long when, int modifiers, int x, int y, int clickCount, boolean popup, int button) {
        super(source, id, when, modifiers);
        this.x = x; this.y = y; this.clickCount = clickCount; this.button = button;
    }

    public int getX() { return x; }
    public int getY() { return y; }
    public Point getPoint() { return new Point(x, y); }
    public int getButton() { return button; }
    public int getClickCount() { return clickCount; }
}
