package websim;

/** Entry point: starts the key pump and the DataStore saver, then boots DangerOS. */
public final class Main {
    public static void main(String[] args) throws Exception {
        Thread pump = new Thread(new danger.ui.WebEventPump(), "WebEventPump");
        pump.setDaemon(true);
        pump.start();
        Thread saver = new Thread(new StoreSaver(), "StoreSaver");
        saver.setDaemon(true);
        saver.start();
        danger.Boot.main(args);
    }
}
