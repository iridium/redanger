package java.awt;

public class Window extends Container {
    public void toFront() {}
    public void toBack() {}
    public void pack() {}
    public void dispose() { visible = false; }
    public boolean isFocused() { return true; }
    protected void processWindowEvent(java.awt.event.WindowEvent e) {}
}
