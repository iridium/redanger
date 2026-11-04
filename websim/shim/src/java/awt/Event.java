package java.awt;

/** The Java 1.0 event, used by Frame.action(). */
public class Event implements java.io.Serializable {
    public Object target;
    public long when;
    public int id, x, y, key, modifiers, clickCount;
    public Object arg;

    public Event(Object target, int id, Object arg) { this.target = target; this.id = id; this.arg = arg; }
}
