package danger.audio;

/**
 * A Stream played by the page with Web Audio (which decodes MP3, WAV, etc.). The 3.4
 * simulator had no Stream implementation at all; the host returns one of these from
 * SimAudioManagerImpl.createStream. State lives on the JS side, keyed by id.
 */
public class WebStream extends Stream {
    private final int id = create();
    private boolean loaded, looping, paused, doneFired;
    private int volume = 100;
    private int loops;

    public WebStream() {
        // the device's native code registers each Stream; destroy() unregisters it
        AudioManager.addAudioObject(this);
    }

    static native int create();
    static native boolean loadFile(int id, String path);
    static native boolean loadBytes(int id, byte[] data);
    static native void play(int id, int offsetMs, boolean loop);
    static native void halt(int id, boolean pause);
    static native void setGain(int id, int volume);
    static native void free(int id);
    /** {playing, done, positionMs, lengthMs} */
    static native int[] state(int id);

    public boolean loadFromFile(byte[] data) { return loaded = data != null && loadBytes(id, data); }
    public boolean loadFromFileName(String path) { return loaded = path != null && loadFile(id, path); }
    public boolean isLoaded() { return loaded; }

    public boolean startDefault() { return startAtOffset(0); }
    public boolean startWithFilter(int filter) { return startAtOffset(0); }
    public boolean start(int offset, int filter) { return startAtOffset(offset); }

    public boolean startAtOffset(int offsetMs) {
        if (!loaded) return false;
        paused = false;
        doneFired = false;
        play(id, offsetMs, looping || loops > 1);
        return true;
    }

    public void stop() {
        paused = false;
        halt(id, false);
        super.stop();
    }

    public void pause() { paused = true; halt(id, true); }
    public void resume() { if (paused) { paused = false; play(id, -1, looping); } }
    public boolean isPaused() { return paused; }

    public boolean isPlaying() { return state(id)[0] != 0; }

    public boolean isDone() {
        boolean done = state(id)[1] != 0;
        if (done && !doneFired) {
            doneFired = true;
            doDoneCallbackEvent();
        }
        return done;
    }

    public void destroy() {
        super.destroy();
        free(id);
    }

    public void setLoopingEnabled(boolean on) { looping = on; }
    public boolean isLoopingEnabled() { return looping; }
    public void setLoops(int n) { loops = n; looping = n != 1; }
    public int getLoops() { return loops; }
    public void setLoopCount(int n) { setLoops(n); }
    public int getLoopCount() { return loops; }

    public void setVolume(int v) { volume = v; setGain(id, v); }
    public int getVolume() { return volume; }

    public int getMillisecondLength() { return state(id)[3]; }
    public int getMillisecondPosition() { return state(id)[2]; }
    public void setMillisecondPosition(int ms) { if (isPlaying()) play(id, ms, looping); }
    public int getMicrosecondLength() { return getMillisecondLength() * 1000; }
    public int getMicrosecondPosition() { return getMillisecondPosition() * 1000; }
    public void setMicrosecondPosition(int us) { setMillisecondPosition(us / 1000); }
    public int getPlaybackLength() { return getMillisecondLength(); }
    public int getPlaybackPosition() { return getMillisecondPosition(); }
    public void setPlaybackPosition(int ms) { setMillisecondPosition(ms); }

    public void setRouteBus(int bus) {}
    public void setPriority(int p) {}
    public int getPriority() { return 0; }
    public void setRate(int r) {}
    public int getRate() { return 22050; }
    public void setBuzzFlashEnable(boolean buzz, boolean flash) {}
    public void setBuzzFlash(boolean buzz, boolean flash) {}
    public void fadeFromToInTime(int from, int to, int ms) { setVolume(to); }
}
