package java.awt;

import java.awt.image.ImageObserver;

public abstract class Image {
    public abstract int getWidth(ImageObserver o);
    public abstract int getHeight(ImageObserver o);
    public abstract Graphics getGraphics();
    public void flush() {}
}
