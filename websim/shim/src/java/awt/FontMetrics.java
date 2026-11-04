package java.awt;

public class FontMetrics implements java.io.Serializable {
    protected Font font;
    protected FontMetrics(Font f) { font = f; }
    public Font getFont() { return font; }
    public int getAscent() { return Math.round(font.size * 0.8f); }
    public int getDescent() { return Math.round(font.size * 0.2f); }
    public int getHeight() { return getAscent() + getDescent(); }
    public int charWidth(int c) { return Math.round(font.size * 0.6f); }
    public int charWidth(char c) { return charWidth((int) c); }
    public int stringWidth(String s) { return s.length() * charWidth('m'); }
}
