package websim;

/** The application class loader: everything lives on the VM's boot class path. */
public final class SystemLoader extends ClassLoader {
    public SystemLoader() { super(null); }

    protected Class<?> loadClass(String name, boolean resolve) throws ClassNotFoundException {
        return Class.forName(name, false, null);
    }

    protected Class<?> findClass(String name) throws ClassNotFoundException {
        return Class.forName(name, false, null);
    }
}
