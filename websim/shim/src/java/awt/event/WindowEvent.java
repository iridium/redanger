package java.awt.event;

public class WindowEvent extends java.util.EventObject {
    public static final int WINDOW_CLOSING = 201;
    int id;
    public WindowEvent(Object source, int id) { super(source); this.id = id; }
    public int getID() { return id; }
}
