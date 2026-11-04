import java.util.*;

public class Hello {
    static int counter;

    public static void main(String[] args) throws Exception {
        System.out.println("Hello from websim " + 42 + " " + 3.5 + " " + (1L << 40) + " " + 'c');
        Map<String, Integer> m = new HashMap<>();
        for (int i = 0; i < 100; i++) m.put("k" + i, i * i);
        System.out.println("map " + m.size() + " k7=" + m.get("k7"));
        Vector v = new Vector(); v.addElement("a"); v.addElement("b");
        Hashtable h = new Hashtable(); h.put("x", "y");
        System.out.println(v + " " + h + " " + Integer.toHexString(-1) + " " + Long.MAX_VALUE + " " + Math.sqrt(2));
        try { Object o = null; o.hashCode(); } catch (NullPointerException e) { System.out.println("caught " + e); }
        try { int[] a = new int[2]; a[5] = 1; } catch (ArrayIndexOutOfBoundsException e) { System.out.println("caught " + e.getMessage()); }
        final Object lock = new Object();
        Thread th = new Thread(new Runnable() { public void run() {
            synchronized (lock) { counter++; lock.notifyAll(); }
            System.out.println("thread ran: " + Thread.currentThread().getName());
        }});
        synchronized (lock) { th.start(); while (counter == 0) lock.wait(); }
        th.join();
        Thread.sleep(20);
        String s = String.format("%05d|%s|%.2f", 42, "fmt", 1.5);
        System.out.println(s + " " + "abc".getBytes("UTF-8").length + " " + new String(new byte[] {104, 105}, "UTF-8"));
        StringBuffer sb = new StringBuffer(); for (int i = 0; i < 5; i++) sb.append(i); System.out.println(sb.reverse());
        switch ("two") { case "one": System.out.println(1); break; case "two": System.out.println(2); break; }
        System.out.println(Class.forName("java.util.ArrayList").newInstance().getClass().getName());
        new Throwable("trace test").printStackTrace();
    }
}
