package java.awt;

public class MenuItem extends MenuComponent {
    String label;
    boolean enabled = true;
    public MenuItem(String label) { this.label = label; }
    public String getLabel() { return label; }
    public void setLabel(String l) { label = l; }
    public void setEnabled(boolean b) { enabled = b; }
}
