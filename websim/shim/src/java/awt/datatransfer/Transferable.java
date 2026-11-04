package java.awt.datatransfer;

public interface Transferable {
    Object getTransferData(DataFlavor flavor) throws UnsupportedFlavorException, java.io.IOException;
}
