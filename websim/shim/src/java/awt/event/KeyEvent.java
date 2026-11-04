package java.awt.event;

public class KeyEvent extends InputEvent {
    public static final int KEY_TYPED = 400, KEY_PRESSED = 401, KEY_RELEASED = 402;
    public static final char CHAR_UNDEFINED = 0xffff;
    int keyCode;
    char keyChar;

    public KeyEvent(Object source, int id, long when, int modifiers, int keyCode, char keyChar) {
        super(source, id, when, modifiers);
        this.keyCode = keyCode;
        this.keyChar = keyChar;
    }

    public int getKeyCode() { return keyCode; }
    public char getKeyChar() { return keyChar; }
    public void setKeyChar(char c) { keyChar = c; }
    public String toString() { return "KeyEvent[" + id + "," + keyCode + "," + (int) keyChar + "]"; }
}
