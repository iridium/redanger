package java.awt;

public final class AlphaComposite implements Composite {
    public static final int CLEAR = 1, SRC = 2, SRC_OVER = 3, DST_OVER = 4, SRC_IN = 5, DST_IN = 6,
        SRC_OUT = 7, DST_OUT = 8, DST = 9, SRC_ATOP = 10, DST_ATOP = 11, XOR = 12;
    public static final AlphaComposite Clear = new AlphaComposite(CLEAR, 1f);
    public static final AlphaComposite Src = new AlphaComposite(SRC, 1f);
    public static final AlphaComposite SrcOver = new AlphaComposite(SRC_OVER, 1f);

    final int rule;
    final float alpha;

    private AlphaComposite(int rule, float alpha) { this.rule = rule; this.alpha = alpha; }

    public static AlphaComposite getInstance(int rule) { return getInstance(rule, 1f); }
    public static AlphaComposite getInstance(int rule, float alpha) {
        if (alpha == 1f) {
            if (rule == SRC_OVER) return SrcOver;
            if (rule == SRC) return Src;
            if (rule == CLEAR) return Clear;
        }
        return new AlphaComposite(rule, alpha);
    }
    public int getRule() { return rule; }
    public float getAlpha() { return alpha; }
}
