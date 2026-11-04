package java.awt;

public class MenuBar extends MenuComponent {
    final java.util.Vector menus = new java.util.Vector();
    public MenuBar() {}
    public Menu add(Menu m) { menus.addElement(m); return m; }
    public void setHelpMenu(Menu m) { add(m); }
}
