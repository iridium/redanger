package java.awt;

public class Frame extends Window {
    String title;
    MenuBar menuBar;

    public Frame() { this(""); }
    public Frame(String title) { this.title = title; }
    public String getTitle() { return title; }
    public void setTitle(String t) { title = t; }
    public void setResizable(boolean b) {}
    public void setMenuBar(MenuBar mb) { menuBar = mb; }
    public MenuBar getMenuBar() { return menuBar; }
    public void setIconImage(Image i) {}
}
