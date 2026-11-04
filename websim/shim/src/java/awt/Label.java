package java.awt;

public class Label extends Component {
    String text;
    public Label(String text) { this.text = text; }
    public String getText() { return text; }
    public void setText(String t) { text = t; }
}
