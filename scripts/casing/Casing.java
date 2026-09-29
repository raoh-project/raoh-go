// Prints how Java's String.toUpperCase and toLowerCase with Locale.ROOT map each code point on its
// own, for every code point they change. internal/gencase reads the output and keeps the entries
// where Go's unicode.ToUpper and unicode.ToLower give something else.
//
// Run through scripts/casing/generate.sh.

import java.util.Locale;

public class Casing {
    public static void main(String[] args) {
        // Each line is the direction (U or L), the code point and what it maps to, in hexadecimal.
        System.out.println("# Java " + Runtime.version());
        for (int cp = 0; cp <= Character.MAX_CODE_POINT; cp++) {
            if (Character.getType(cp) == Character.SURROGATE || !Character.isDefined(cp)) {
                continue;
            }
            String s = new String(Character.toChars(cp));
            print("U", cp, s, s.toUpperCase(Locale.ROOT));
            print("L", cp, s, s.toLowerCase(Locale.ROOT));
        }
    }

    static void print(String direction, int cp, String from, String to) {
        if (from.equals(to)) {
            return;
        }
        var out = new StringBuilder(direction).append('\t').append(Integer.toHexString(cp)).append('\t');
        to.codePoints().forEach(c -> out.append(Integer.toHexString(c)).append(' '));
        System.out.println(out.toString().trim());
    }
}
