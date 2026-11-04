package java.awt;

public class Menu extends MenuItem {
    final java.util.Vector items = new java.util.Vector();
    public Menu(String label) { super(label); }
    public MenuItem add(MenuItem mi) { items.addElement(mi); return mi; }
    public void add(String label) { add(new MenuItem(label)); }
    public void addSeparator() {}
    public void removeAll() { items.removeAllElements(); }
    public int getItemCount() { return items.size(); }
}
