package websim;

import danger.app.DataStore;

/**
 * Writes the DataStores (settings, messages, game saves) to ./datastores a couple of seconds
 * after any of them changes. The simulator reads them back at boot (SimHeap) but only writes
 * them at power-off, which a closed browser tab never reaches. The worker keeps /state in
 * IndexedDB, so this is what makes them survive a reload.
 */
final class StoreSaver implements Runnable {
    public void run() {
        boolean retry = false;
        while (true) {
            try { Thread.sleep(2000); } catch (InterruptedException e) { return; }
            if (!WebHost.storesChanged() && !retry) continue;
            try {
                DataStore.flattenStores();
                retry = false;
            } catch (Throwable t) {   // a store changed under us, or the system isn't up yet
                retry = true;
            }
        }
    }
}
