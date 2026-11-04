package java.awt;

public class TextField extends Component {
    String text = "";
    public TextField(int columns) {}
    public TextField(String text) { this.text = text; }
    public String getText() { return text; }
    public void setText(String t) { text = t; }
}
