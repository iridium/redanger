package java.awt;

public class Color implements java.io.Serializable {
    public static final Color white = new Color(255, 255, 255), WHITE = white;
    public static final Color lightGray = new Color(192, 192, 192), LIGHT_GRAY = lightGray;
    public static final Color gray = new Color(128, 128, 128), GRAY = gray;
    public static final Color darkGray = new Color(64, 64, 64), DARK_GRAY = darkGray;
    public static final Color black = new Color(0, 0, 0), BLACK = black;
    public static final Color red = new Color(255, 0, 0), RED = red;
    public static final Color pink = new Color(255, 175, 175), PINK = pink;
    public static final Color orange = new Color(255, 200, 0), ORANGE = orange;
    public static final Color yellow = new Color(255, 255, 0), YELLOW = yellow;
    public static final Color green = new Color(0, 255, 0), GREEN = green;
    public static final Color magenta = new Color(255, 0, 255), MAGENTA = magenta;
    public static final Color cyan = new Color(0, 255, 255), CYAN = cyan;
    public static final Color blue = new Color(0, 0, 255), BLUE = blue;

    int value;

    public Color(int r, int g, int b) { this(r, g, b, 255); }
    public Color(int r, int g, int b, int a) { value = ((a & 255) << 24) | ((r & 255) << 16) | ((g & 255) << 8) | (b & 255); }
    public Color(int rgb) { value = 0xff000000 | rgb; }
    public Color(int argb, boolean hasAlpha) { value = hasAlpha ? argb : 0xff000000 | argb; }
    public Color(float r, float g, float b) { this((int) (r * 255 + 0.5), (int) (g * 255 + 0.5), (int) (b * 255 + 0.5)); }
    public Color(float r, float g, float b, float a) { this((int) (r * 255 + 0.5), (int) (g * 255 + 0.5), (int) (b * 255 + 0.5), (int) (a * 255 + 0.5)); }

    public int getRGB() { return value; }
    public int getRed() { return (value >> 16) & 255; }
    public int getGreen() { return (value >> 8) & 255; }
    public int getBlue() { return value & 255; }
    public int getAlpha() { return (value >>> 24) & 255; }
    public Color brighter() { return new Color(Math.min(255, getRed() * 10 / 7), Math.min(255, getGreen() * 10 / 7), Math.min(255, getBlue() * 10 / 7)); }
    public Color darker() { return new Color(getRed() * 7 / 10, getGreen() * 7 / 10, getBlue() * 7 / 10); }
    public boolean equals(Object o) { return o instanceof Color && ((Color) o).value == value; }
    public int hashCode() { return value; }
    public String toString() { return "java.awt.Color[r=" + getRed() + ",g=" + getGreen() + ",b=" + getBlue() + "]"; }

    public static Color getHSBColor(float h, float s, float b) { return new Color(HSBtoRGB(h, s, b)); }

    public static int HSBtoRGB(float hue, float saturation, float brightness) {
        int r = 0, g = 0, b = 0;
        if (saturation == 0) {
            r = g = b = (int) (brightness * 255.0f + 0.5f);
        } else {
            float h = (hue - (float) Math.floor(hue)) * 6.0f;
            float f = h - (float) Math.floor(h);
            float p = brightness * (1.0f - saturation);
            float q = brightness * (1.0f - saturation * f);
            float t = brightness * (1.0f - (saturation * (1.0f - f)));
            switch ((int) h) {
                case 0: r = (int) (brightness * 255 + 0.5f); g = (int) (t * 255 + 0.5f); b = (int) (p * 255 + 0.5f); break;
                case 1: r = (int) (q * 255 + 0.5f); g = (int) (brightness * 255 + 0.5f); b = (int) (p * 255 + 0.5f); break;
                case 2: r = (int) (p * 255 + 0.5f); g = (int) (brightness * 255 + 0.5f); b = (int) (t * 255 + 0.5f); break;
                case 3: r = (int) (p * 255 + 0.5f); g = (int) (q * 255 + 0.5f); b = (int) (brightness * 255 + 0.5f); break;
                case 4: r = (int) (t * 255 + 0.5f); g = (int) (p * 255 + 0.5f); b = (int) (brightness * 255 + 0.5f); break;
                case 5: r = (int) (brightness * 255 + 0.5f); g = (int) (p * 255 + 0.5f); b = (int) (q * 255 + 0.5f); break;
            }
        }
        return 0xff000000 | (r << 16) | (g << 8) | b;
    }
}
