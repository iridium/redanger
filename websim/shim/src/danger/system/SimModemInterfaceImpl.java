package danger.system;

import danger.io.ATInterfaceImpl;
import java.io.InputStream;
import java.io.OutputStream;

/**
 * The simulator's modem link, which normally connects to a TCP port. Here the
 * modem is answered in memory instead (like references/sdk/fakemodem.py): just
 * enough AT replies for the SMS stack to come up, so Messaging can load its
 * inbox. Selected by -Dcom.danger.ATInterface.ModemPort (any non-zero value).
 */
public class SimModemInterfaceImpl implements ATInterfaceImpl {
    private static final Object sLock = new Object();
    private static final StringBuffer sCmd = new StringBuffer();
    private static byte[] sOut = new byte[256];
    private static int sHead, sTail;

    public SimModemInterfaceImpl() {}

    public void open(int i) {}
    public void destroy() {}
    public void setReadTimeout(int i) {}
    public void close() {}

    public InputStream getInputStream() {
        return new InputStream() {
            public int read() {
                synchronized (sLock) {
                    while (sHead == sTail) {
                        try { sLock.wait(); } catch (InterruptedException e) { return -1; }
                    }
                    return sOut[sHead++] & 0xff;
                }
            }

            public int read(byte[] b, int off, int len) {
                if (len == 0) return 0;
                synchronized (sLock) {
                    int c = read();
                    b[off] = (byte) c;
                    int n = Math.min(len - 1, sTail - sHead);
                    System.arraycopy(sOut, sHead, b, off + 1, n);
                    sHead += n;
                    return n + 1;
                }
            }

            public int available() {
                synchronized (sLock) { return sTail - sHead; }
            }
        };
    }

    public OutputStream getOutputStream() {
        return new OutputStream() {
            public void write(int b) {
                synchronized (sLock) {
                    if (b == '\r') {
                        String cmd = sCmd.toString().trim();
                        sCmd.setLength(0);
                        if (cmd.length() > 0) reply(answer(cmd));
                    } else if (b != '\n') {
                        sCmd.append((char) (b & 0xff));
                    }
                }
            }
        };
    }

    private static void reply(String s) {
        int n = s.length();
        if (sTail + n > sOut.length) {
            byte[] o = new byte[Math.max(sOut.length * 2, sTail - sHead + n)];
            System.arraycopy(sOut, sHead, o, 0, sTail - sHead);
            sTail -= sHead;
            sHead = 0;
            sOut = o;
        }
        for (int i = 0; i < n; i++) sOut[sTail++] = (byte) s.charAt(i);
        sLock.notifyAll();
    }

    private static String answer(String cmd) {
        String c = cmd.toUpperCase();
        String r;
        if (c.equals("AT+CSMS=?")) r = "+CSMS: (0,1)";
        else if (c.equals("AT+CPMS=?")) r = "+CPMS: (\"SM\",\"ME\"),(\"SM\",\"ME\"),(\"SM\",\"ME\")";
        else if (c.equals("AT+CGSN")) r = "351234567890123";
        else if (c.equals("AT+CIMI")) r = "310260000000000";
        else if (c.equals("AT+CSQ")) r = "+CSQ: 20,0";
        else if (c.equals("AT+CREG?")) r = "+CREG: 0,1";
        else if (c.equals("AT+CGREG?")) r = "+CGREG: 0,1";
        else if (c.equals("AT+COPS?")) r = "+COPS: 0,0,\"T-Mobile\"";
        else if (c.equals("AT+CPIN?")) r = "+CPIN: READY";
        else if (c.equals("AT+CNUM")) r = "+CNUM: \"\",\"5550100\",129";
        else if (c.equals("AT+CPMS?")) r = "+CPMS: \"SM\",0,30,\"SM\",0,30,\"SM\",0,30";
        else if (c.equals("AT+CGMR")) r = "REDANGER_WEBSIM_MODEM";
        else if (c.startsWith("AT+CSMS=")) r = "+CSMS: 1,1,1";
        else if (c.startsWith("AT+CPMS=")) r = "+CPMS: 0,30,0,30,0,30";
        else if (c.startsWith("AT+CMGR")) return "\r\n+CMS ERROR: 321\r\n"; // nothing stored on the SIM
        else r = "";
        return (r.length() > 0 ? "\r\n" + r + "\r\n" : "") + "\r\nOK\r\n";
    }
}
