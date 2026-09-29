// Prints the data Unicode normalization is driven by, as Java's java.text.Normalizer has it: the
// canonical combining class of each code point, its full canonical and compatibility
// decompositions, and the primary composites. internal/gennorm reads the output and writes
// internal/norm/tables.go.
//
// Java gives no public access to the combining class, so this reads it from the JDK's internal
// UCharacter. Only this generator depends on it; nothing generated does. Run through
// scripts/normalization/generate.sh, which passes the --add-exports it needs.
//
// Each line is a tab-separated record; code points are hexadecimal, and a sequence of them is
// separated by spaces.
//   C  cp  ccc          a code point whose combining class is not 0
//   D  cp  seq          its full canonical decomposition, when it has one
//   K  cp  seq          its full compatibility decomposition, when that is not the canonical one
//   P  first second cp  a primary composite: cp is what first and second compose to
// Hangul syllables are left out; they are decomposed and composed by rule.

import java.text.Normalizer;
import java.text.Normalizer.Form;
import jdk.internal.icu.lang.UCharacter;
import jdk.internal.icu.util.VersionInfo;

public class Norm {
    public static void main(String[] args) {
        if (Runtime.version().feature() != 25) {
            throw new IllegalStateException("Needs Java 25, not " + Runtime.version());
        }
        // The version has no accessor, so it is compared with the one this generator is for.
        if (UCharacter.getUnicodeVersion().compareTo(VersionInfo.getInstance(16, 0, 0, 0)) != 0) {
            throw new IllegalStateException("Needs Unicode 16.0, not " + UCharacter.getUnicodeVersion());
        }
        System.out.println("# Java " + Runtime.version() + ", Unicode 16.0");
        for (int cp = 0; cp <= Character.MAX_CODE_POINT; cp++) {
            if (Character.getType(cp) == Character.SURROGATE || isHangulSyllable(cp)) {
                continue;
            }
            String s = new String(Character.toChars(cp));
            int ccc = UCharacter.getCombiningClass(cp);
            if (ccc != 0) {
                System.out.println("C\t" + hex(cp) + "\t" + ccc);
            }
            String nfd = Normalizer.normalize(s, Form.NFD);
            String nfkd = Normalizer.normalize(s, Form.NFKD);
            if (!nfd.equals(s)) {
                System.out.println("D\t" + hex(cp) + "\t" + seq(nfd));
                pair(cp, nfd);
            }
            if (!nfkd.equals(nfd)) {
                System.out.println("K\t" + hex(cp) + "\t" + seq(nfkd));
            }
        }
    }

    // A character with a canonical decomposition is either a primary composite or excluded from
    // composition. Composing its decomposition back tells which; a primary composite must then
    // be the composition of one code point and the last one of the decomposition.
    static void pair(int cp, String nfd) {
        if (!Normalizer.normalize(nfd, Form.NFC).equals(new String(Character.toChars(cp)))) {
            return;
        }
        int[] d = nfd.codePoints().toArray();
        if (d.length < 2) {
            throw new IllegalStateException(hex(cp) + " is composed from a single code point");
        }
        int last = d[d.length - 1];
        int[] head = java.util.Arrays.copyOf(d, d.length - 1);
        String first = Normalizer.normalize(new String(head, 0, head.length), Form.NFC);
        if (first.codePointCount(0, first.length()) != 1) {
            throw new IllegalStateException(hex(cp) + " is not the composition of one code point and " + hex(last));
        }
        int f = first.codePointAt(0);
        String again = Normalizer.normalize(first + new String(Character.toChars(last)), Form.NFC);
        if (!again.equals(new String(Character.toChars(cp)))) {
            throw new IllegalStateException(hex(f) + " and " + hex(last) + " do not compose to " + hex(cp));
        }
        System.out.println("P\t" + hex(f) + "\t" + hex(last) + "\t" + hex(cp));
    }

    static boolean isHangulSyllable(int cp) {
        return cp >= 0xAC00 && cp <= 0xD7A3;
    }

    static String hex(int cp) {
        return Integer.toHexString(cp);
    }

    static String seq(String s) {
        var out = new StringBuilder();
        s.codePoints().forEach(c -> out.append(out.length() == 0 ? "" : " ").append(hex(c)));
        return out.toString();
    }
}
