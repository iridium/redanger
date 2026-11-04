package java.awt;

import java.awt.image.BufferedImage;

public class Graphics2D extends Graphics {
    public Graphics2D(BufferedImage target) { super(target); }
    public Composite getComposite() { return composite; }
    public void setComposite(Composite c) { composite = c; }
}
