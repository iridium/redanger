package java.awt.event;

public abstract class InputEvent extends java.util.EventObject {
    public static final int SHIFT_MASK = 1, CTRL_MASK = 2, META_MASK = 4, ALT_MASK = 8, BUTTON1_MASK = 16;
    int id;
    long when;
    int modifiers;

    InputEvent(Object source, int id, long when, int modifiers) {
        super(source);
        this.id = id;
        this.when = when;
        this.modifiers = modifiers;
    }

    public int getID() { return id; }
    public long getWhen() { return when; }
    public int getModifiers() { return modifiers; }
    public boolean isShiftDown() { return (modifiers & SHIFT_MASK) != 0; }
    public boolean isControlDown() { return (modifiers & CTRL_MASK) != 0; }
    public boolean isAltDown() { return (modifiers & ALT_MASK) != 0; }
    public void consume() {}
}
