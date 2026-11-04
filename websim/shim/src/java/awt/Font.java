package java.awt;

import java.io.InputStream;

/** TrueType fonts are not rendered in the browser simulator; this only carries a name and size. */
public class Font implements java.io.Serializable {
    public static final int PLAIN = 0, BOLD = 1, ITALIC = 2, TRUETYPE_FONT = 0;
    protected String name;
    protected int style;
    protected float size;

    public Font(String name, int style, int size) { this.name = name; this.style = style; this.size = size; }
    public static Font createFont(int format, InputStream in) throws FontFormatException { return new Font("TrueType", PLAIN, 1); }
    public Font deriveFont(int style, float size) { Font f = new Font(name, style, (int) size); f.size = size; return f; }
    public Font deriveFont(float size) { return deriveFont(style, size); }
    public String getFamily() { return name; }
    public String getFontName() { return name; }
    public String getName() { return name; }
    public int getSize() { return (int) size; }
    public int getStyle() { return style; }
}
