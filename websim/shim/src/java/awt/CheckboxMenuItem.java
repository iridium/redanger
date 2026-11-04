package java.awt;

public class CheckboxMenuItem extends MenuItem {
    boolean state;
    public CheckboxMenuItem(String label) { super(label); }
    public CheckboxMenuItem(String label, boolean state) { super(label); this.state = state; }
    public boolean getState() { return state; }
    public void setState(boolean b) { state = b; }
}
