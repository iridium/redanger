package java.awt.event;

public class MouseWheelEvent extends MouseEvent {
    int rotation;

    public MouseWheelEvent(Object source, int id, long when, int modifiers, int x, int y, int clickCount, boolean popup, int type, int amount, int rotation) {
        super(source, id, when, modifiers, x, y, clickCount, popup, 0);
        this.rotation = rotation;
    }

    public int getWheelRotation() { return rotation; }
}
