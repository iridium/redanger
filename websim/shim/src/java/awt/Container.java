package java.awt;

public class Container extends Component {
    public Insets getInsets() { return new Insets(0, 0, 0, 0); }
    public Component add(Component c) { c.parent = this; return c; }
    public Component add(String name, Component c) { return add(c); }
    public void remove(Component c) {}
    public void setLayout(LayoutManager m) {}
    public void doLayout() {}
}
