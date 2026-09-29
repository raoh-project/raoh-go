// Runs every case of testdata/compat/cases.json through Raoh for Java and writes what it gives to
// testdata/compat/expected.json. Each decoder here has a counterpart of the same name in
// compat_test.go; the Go tests hold the Go decoders to what these ones gave.
//
// Run through scripts/compat/generate.sh.

import static net.unit8.raoh.json.JsonDecoders.*;

import java.math.BigDecimal;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.regex.Pattern;
import net.unit8.raoh.Err;
import net.unit8.raoh.Issue;
import net.unit8.raoh.Ok;
import net.unit8.raoh.Presence;
import net.unit8.raoh.Result;
import net.unit8.raoh.decode.Decoder;
import net.unit8.raoh.decode.Decoders;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.json.JsonMapper;

public class Generate {

    enum Color { RED, GREEN }

    // Map.of iterates in an order that changes from run to run; sorting the keys keeps the
    // generated file the same when nothing it records has changed.
    static final JsonMapper MAPPER = JsonMapper.builder()
            .enable(tools.jackson.databind.SerializationFeature.ORDER_MAP_ENTRIES_BY_KEYS)
            .build();

    static Decoder<JsonNode, ?> decoder(String name) {
        return switch (name) {
            case "string" -> string();
            case "string_normalized_email" -> string().trim().toLowerCase().email();
            case "string_non_blank" -> string().nonBlank();
            case "string_min_3" -> string().minLength(3);
            case "string_max_3" -> string().maxLength(3);
            case "string_length_4" -> string().fixedLength(4);
            case "string_one_of" -> string().oneOf("b", "a");
            case "string_starts_with" -> string().startsWith("ab");
            case "string_ends_with" -> string().endsWith("ab");
            case "string_contains" -> string().includes("ab");
            case "string_ipv4" -> string().ipv4();
            case "string_ipv6" -> string().ipv6();
            case "string_ip" -> string().ip();
            case "string_ulid" -> string().ulid();
            case "string_cuid" -> string().cuid();
            case "string_pattern" -> string().pattern(Pattern.compile("[a-z]+\\d"));
            case "string_trim" -> string().trim();
            case "string_lower" -> string().toLowerCase();
            case "string_upper" -> string().toUpperCase();
            case "string_email" -> string().email();
            case "string_one_of_astral" -> string().oneOf("\uff21", "\ud83d\ude00");
            case "string_uuid" -> string().uuid().map(Object::toString);
            case "string_url" -> string().url().map(Object::toString);
            case "string_uri" -> string().uri().map(Object::toString);
            case "string_to_int" -> string().toInt();
            case "string_to_int_min_1" -> string().toInt().min(1);
            case "string_to_long" -> string().toLong();
            case "string_to_long_positive" -> string().toLong().positive();
            case "string_max_3_to_int_message" -> string().maxLength(3).toInt("bad");
            case "string_to_decimal" -> string().toDecimal();
            case "string_to_decimal_scale_2" -> string().toDecimal().scale(2);
            case "string_to_decimal_positive" -> string().toDecimal().positive();
            case "string_to_bool" -> string().toBool();
            case "string_to_bool_is_true" -> string().toBool().isTrue();

            case "int" -> int_();
            case "int_min_1" -> int_().min(1);
            case "int_max_10" -> int_().max(10);
            case "int_range" -> int_().range(0, 150);
            case "int_positive" -> int_().positive();
            case "int_negative" -> int_().negative();
            case "int_non_negative" -> int_().nonNegative();
            case "int_non_positive" -> int_().nonPositive();
            case "int_multiple_of_3" -> int_().multipleOf(3);
            case "int_one_of" -> int_().oneOf(3, 1);
            case "long" -> long_();

            case "double" -> double_();
            case "double_positive" -> double_().positive();
            case "double_negative" -> double_().negative();
            case "double_range" -> double_().range(0.5, 1.5);
            case "double_min" -> double_().min(0.5);
            case "double_one_of" -> double_().oneOf(2.0, 1.0);
            case "double_min_1e7" -> double_().min(1e7);
            case "double_max_small" -> double_().max(1e-4);
            case "double_one_of_big" -> double_().oneOf(1e7, 0.5);

            case "decimal" -> decimal();
            case "decimal_scale_2" -> decimal().scale(2);
            case "decimal_positive" -> decimal().positive();
            case "decimal_range" -> decimal().range(new BigDecimal("0"), new BigDecimal("10"));
            case "decimal_min_small" -> decimal().min(new BigDecimal("0.0005"));

            case "bool" -> bool();
            case "bool_is_true" -> bool().isTrue();

            case "list_int" -> list(int_());
            case "list_non_empty" -> list(int_()).nonempty();
            case "list_min_2" -> list(int_()).minSize(2);
            case "list_max_2" -> list(int_()).maxSize(2);
            case "list_size_2" -> list(int_()).fixedSize(2);
            case "list_unique" -> list(int_()).unique();

            case "person" -> combine(field("name", string()), field("age", int_()))
                    .map((n, a) -> List.of(n, a));
            case "person_strict" -> combine(field("name", string()), field("age", int_()))
                    .strict((n, a) -> List.of(n, a));
            case "escaped_keys" -> combine(field("a/b", int_()), field("~c", int_()))
                    .map((a, b) -> List.of(a, b));
            case "optional" -> combine(field("id", int_()), optionalField("nick", string()))
                    .map((id, nick) -> listOf(id, nick.orElse(null)));
            case "nullable" -> combine(field("id", int_()), field("note", nullable(string())))
                    .map((id, note) -> listOf(id, note));
            case "presence" -> combine(field("id", int_()), optionalNullableField("n", int_()))
                    .map((id, n) -> listOf(id, presence(n)));
            case "nested" -> combine(
                    field("items", list(combine(
                            field("name", string().nonBlank()),
                            field("qty", int_().positive()))
                            .map((n, q) -> List.of(n, q)))),
                    field("count", Decoders.withDefault(int_(), 0)))
                    .map((items, count) -> List.of(items, count));
            case "dict" -> map(int_());
            case "optional_only" -> combine(optionalField("a", string()), optionalField("b", string()))
                    .map((a, b) -> listOf(a.orElse(null), b.orElse(null)));

            case "enum" -> enumOf(Color.class).map(c -> c.name().toLowerCase());
            case "literal" -> literal("v1");
            case "shape" -> discriminate("kind",
                    variant("square", combine(field("side", int_()), field("kind", string()))
                            .map((s, k) -> s * s)),
                    variant("rect", combine(field("w", int_()), field("h", int_()))
                            .map((w, h) -> w * h)));
            case "one_of" -> Decoders.<JsonNode, String>oneOf(
                    int_().map(String::valueOf), string().minLength(3));
            case "with_default" -> combine(field("id", Decoders.withDefault(int_(), 0)),
                    field("page", Decoders.withDefault(int_(), 1)))
                    .map((id, page) -> List.of(id, page));
            case "recover" -> combine(field("id", Decoders.withDefault(int_(), 0)),
                    field("page", Decoders.recover(int_(), 1)))
                    .map((id, page) -> List.of(id, page));
            case "period" -> combine(field("start", int_()), field("end", int_()))
                    .flatMap((s, e) -> s <= e
                            ? Result.ok(List.of(s, e))
                            : Result.fail(net.unit8.raoh.Path.ROOT.append("end"),
                                    "invalid_value", "end is before start"));

            case "instant" -> string().iso8601().map(Object::toString);
            case "instant_after" -> string().iso8601()
                    .after(java.time.Instant.parse("2024-01-01T00:00:00Z")).map(Object::toString);
            case "date" -> string().date().map(Object::toString);
            case "date_before" -> string().date()
                    .before(java.time.LocalDate.of(2024, 1, 1)).map(Object::toString);
            case "date_between" -> string().date()
                    .between(java.time.LocalDate.of(2024, 1, 1), java.time.LocalDate.of(2024, 12, 31))
                    .map(Object::toString);
            case "time" -> string().time().map(Object::toString);
            case "time_after" -> string().time()
                    .after(java.time.LocalTime.of(9, 0)).map(Object::toString);
            case "date_time" -> string().dateTime().map(Object::toString);
            case "date_time_before" -> string().dateTime()
                    .before(java.time.LocalDateTime.of(2024, 1, 1, 0, 0)).map(Object::toString);
            case "offset_date_time" -> string().offsetDateTime().map(Object::toString);
            case "offset_date_time_after" -> string().offsetDateTime()
                    .after(java.time.OffsetDateTime.parse("2024-01-01T00:00+09:00")).map(Object::toString);
            default -> throw new IllegalArgumentException("no decoder " + name);
        };
    }

    static List<Object> listOf(Object... values) {
        var list = new ArrayList<Object>();
        for (var v : values) list.add(v);
        return list;
    }

    static Object presence(Presence<?> p) {
        return switch (p) {
            case Presence.Absent<?> _ -> "absent";
            case Presence.PresentNull<?> _ -> "null";
            case Presence.Present<?> present -> Map.of("present", present.value());
        };
    }

    public static void main(String[] args) throws Exception {
        // toLowerCase and toUpperCase follow the default locale; fix it so the output does not
        // depend on the machine that runs this.
        java.util.Locale.setDefault(java.util.Locale.ROOT);
        var cases = MAPPER.readTree(Files.readString(Path.of(args[0])));
        var results = new ArrayList<Map<String, Object>>();
        for (var c : cases) {
            var name = c.get("decoder").asString();
            // A number whose text matters, such as -0, is given as the JSON text to read.
            var text = c.get("input_json");
            var input = text != null ? MAPPER.readTree(text.asString()) : c.get("input");
            var result = new LinkedHashMap<String, Object>();
            result.put("decoder", name);
            if (text != null) {
                result.put("input_json", text.asString());
            } else {
                result.put("input", input);
            }
            Result<?> r = decoder(name).decode(input);
            switch (r) {
                case Ok<?> ok -> result.put("ok", ok.value());
                case Err<?> err -> {
                    var issues = new ArrayList<Map<String, Object>>();
                    for (Issue issue : err.issues().asList()) {
                        var i = new LinkedHashMap<String, Object>();
                        i.put("path", issue.path().toJsonPointer());
                        i.put("code", issue.code());
                        i.put("message_key", issue.messageKey());
                        i.put("message", issue.message());
                        i.put("meta", issue.meta());
                        issues.add(i);
                    }
                    result.put("issues", issues);
                }
            }
            results.add(result);
        }
        Files.writeString(Path.of(args[1]),
                MAPPER.writerWithDefaultPrettyPrinter().writeValueAsString(results) + "\n");
    }
}
