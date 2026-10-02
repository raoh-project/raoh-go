// Runs every case of testdata/compat/cases.json through Raoh for Java and writes what it gives to
// testdata/compat/expected.json. Each decoder here has a counterpart of the same name in
// compat_test.go; the Go tests hold the Go decoders to what these ones gave.
//
// Run through scripts/compat/generate.sh.

import static net.unit8.raoh.json.JsonDecoders.*;

import java.math.BigDecimal;
import java.nio.file.Files;
import java.nio.file.Path;
import java.text.Normalizer;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import net.unit8.raoh.Err;
import net.unit8.raoh.Issue;
import net.unit8.raoh.MessageResolver;
import net.unit8.raoh.Ok;
import net.unit8.raoh.Presence;
import net.unit8.raoh.Result;
import net.unit8.raoh.decode.Decoder;
import net.unit8.raoh.decode.Decoders;
import net.unit8.raoh.decode.ObjectDecoders;
import net.unit8.raoh.encode.MapEncoders;
import net.unit8.raoh.encode.ObjectEncoders;
import net.unit8.raoh.decode.combinator.CombinePart;
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
            case "string_pattern" -> string().pattern("[a-z]+\\d");
            case "string_trim" -> string().trim();
            case "string_lower" -> string().toLowerCase();
            case "string_upper" -> string().toUpperCase();
            case "string_normalize" -> string().normalize();
            case "string_normalize_nfd" -> string().normalize(Normalizer.Form.NFD);
            case "string_normalize_nfkc" -> string().normalize(Normalizer.Form.NFKC);
            case "string_normalize_nfkd" -> string().normalize(Normalizer.Form.NFKD);
            case "string_normalize_max_2" -> string().normalize().maxLength(2);
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
            case "double_non_negative" -> double_().nonNegative();
            case "double_non_positive" -> double_().nonPositive();
            case "double_one_of_zero" -> double_().oneOf(0.0);

            case "float" -> float_();
            case "float_positive" -> float_().positive();
            case "float_negative" -> float_().negative();
            case "float_non_negative" -> float_().nonNegative();
            case "float_non_positive" -> float_().nonPositive();
            case "float_range" -> float_().range(0.5f, 1.5f);
            case "float_min" -> float_().min(0.5f);
            case "float_max" -> float_().max(1.5f);
            case "float_one_of" -> float_().oneOf(2.0f, 1.0f);
            case "float_one_of_zero" -> float_().oneOf(0.0f);
            case "float_min_tenth" -> float_().min(0.1f);
            case "float_min_1e7" -> float_().min(1e7f);
            case "float_max_small" -> float_().max(1e-4f);
            case "property_with_default" -> nullable(string()).map(v ->
                    MapEncoders.<String>object(MapEncoders.<String, String>propertyWithDefault(
                            "value", s -> s, ObjectEncoders.string(), "default")).encode(v));

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
            case "list_contains_2" -> list(int_()).contains(2);
            case "list_contains_all" -> list(int_()).containsAll(1, 3, 3);
            case "list_contains_all_max_2" -> list(int_()).containsAll(1, 3).maxSize(2);
            // A set has no order Go's map keeps, so the elements are compared as a sorted list.
            case "list_to_set" -> list(int_()).toSet().map(s -> s.stream().sorted().toList());
            case "list_max_2_to_set" -> list(int_()).maxSize(2).toSet().map(s -> s.stream().sorted().toList());

            case "person" -> combine(field("name", string()), field("age", int_()))
                    .map((n, a) -> List.of(n, a));
            case "person_strict" -> combine(field("name", string()), field("age", int_()))
                    .strict((n, a) -> List.of(n, a));
            case "flat" -> combine(field("id", int_()), flat(contactDecoder()))
                    .map((id, contact) -> List.of(id, contact));
            case "flat_first" -> combine(flat(contactDecoder()), field("id", int_()))
                    .map((contact, id) -> List.of(contact, id));
            case "flat_nested" -> combine(field("a", int_()),
                    flat(combine(field("b", int_()),
                            flat(combine(field("c", int_()), field("d", int_()))
                                    .map((c, d) -> c * 10 + d)))
                            .map((b, cd) -> b * 100 + cd)))
                    .map((a, bcd) -> a * 1000 + bcd);
            case "object_17" -> combine(List.<CombinePart<JsonNode, ?>>of(
                    field("f1", int_()), field("f2", int_()), field("f3", int_()), field("f4", int_()),
                    field("f5", int_()), field("f6", int_()), field("f7", int_()), field("f8", int_()),
                    field("f9", int_()), field("f10", int_()), field("f11", int_()), field("f12", int_()),
                    field("f13", int_()), field("f14", int_()), field("f15", int_()), field("f16", int_()),
                    field("f17", string())))
                    .map(List::of);
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
                    field("count", withDefault(int_(), 0)))
                    .map((items, count) -> List.of(items, count));
            case "dict" -> map(int_());
            case "dict_non_empty" -> map(int_()).nonempty();
            case "dict_non_empty_message" -> map(int_()).nonempty("empty");
            case "dict_min_2" -> map(int_()).minSize(2);
            case "dict_max_1" -> map(int_()).maxSize(1);
            case "dict_size_2" -> map(int_()).fixedSize(2);
            case "dict_min_2_member_issue" -> map(int_()).minSize(2);
            case "optional_only" -> combine(optionalField("a", string()), optionalField("b", string()))
                    .map((a, b) -> listOf(a.orElse(null), b.orElse(null)));

            case "enum" -> enumOf(Color.class).map(c -> c.name().toLowerCase());
            case "literal" -> literal("v1");
            case "shape" -> discriminate("kind",
                    variant("square", combine(field("side", int_()), field("kind", string()))
                            .map((s, k) -> s * s)),
                    variant("rect", combine(field("w", int_()), field("h", int_()))
                            .map((w, h) -> w * h)));
            case "strict_discriminate" -> strict(discriminate("kind",
                    variant("square", strict(combine(field("side", int_()), flat((JsonNode in, net.unit8.raoh.Path at) -> Result.ok(in)))
                            .map((s, n) -> s * s), java.util.Set.of("kind", "side"))),
                    variant("rect", strict(combine(field("w", int_()), field("h", int_()))
                            .map((w, h) -> w * h), java.util.Set.of("kind", "w", "h")))),
                    java.util.Set.of("kind", "side", "w", "h"));
            case "enum_custom_string" -> Decoders.enumOf(Color.class, string().trim())
                    .map(c -> c.name().toLowerCase());
            case "literal_custom_string" -> Decoders.literal("v1", string().trim().toLowerCase());
            case "discriminate_custom_tag" -> Decoders.discriminate("kind",
                    combine(field("kind", string().trim().toLowerCase()),
                            flat((JsonNode in, net.unit8.raoh.Path at) -> Result.ok(in))).map((k, n) -> k),
                    variant("square", combine(field("side", int_()), field("kind", string()))
                            .map((s, k) -> s * s)),
                    variant("rect", combine(field("w", int_()), field("h", int_()))
                            .map((w, h) -> w * h)));
            case "discriminate_map" -> discriminate("kind", Map.<String, Decoder<JsonNode, ? extends Integer>>of(
                    "square", combine(field("side", int_()), field("kind", string()))
                            .map((s, k) -> s * s),
                    "rect", combine(field("w", int_()), field("h", int_()))
                            .map((w, h) -> w * h)));
            case "one_of" -> Decoders.<JsonNode, String>oneOf(
                    int_().map(String::valueOf), string().minLength(3));
            case "with_default" -> combine(field("id", withDefault(int_(), 0)),
                    field("page", withDefault(int_(), 1)))
                    .map((id, page) -> List.of(id, page));
            case "recover" -> combine(field("id", withDefault(int_(), 0)),
                    field("page", Decoders.recover(int_(), 1)))
                    .map((id, page) -> List.of(id, page));
            case "period" -> combine(field("start", int_()), field("end", int_()))
                    .flatMap((s, e) -> s <= e
                            ? Result.ok(List.of(s, e))
                            : Result.failCustom(net.unit8.raoh.Path.ROOT.append("end"),
                                    "invalid_value", "end is before start", Map.of()));

            case "period_nested" -> combine(field("id", int_()), field("period",
                    combine(field("start", int_()), field("end", int_()))
                            .map((s, e) -> List.of(s, e))
                            .flatMapWithPath((v, at) -> v.get(0) <= v.get(1)
                                    ? Result.ok(v)
                                    : Result.failCustom(at.append("end"),
                                            "invalid_value", "end is before start", Map.of()))))
                    .map((id, period) -> List.of(id, period));
            case "even_meta" -> int_().refine(n -> n % 2 == 0, "must_be_even", "must be even",
                    n -> Map.of("actual", n));
            case "recover_issues" -> combine(field("id", int_()),
                    field("page", Decoders.recover(int_(),
                            (java.util.function.Function<net.unit8.raoh.Issues, Integer>) is -> is.asList().size() + 10)))
                    .map((id, page) -> List.of(id, page));
            case "default_supplier" -> combine(field("id", withDefault(int_(), (java.util.function.Supplier<Integer>) () -> 7)),
                    field("page", withDefault(int_(), (java.util.function.Supplier<Integer>) () -> 8)))
                    .map((id, page) -> List.of(id, page));

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

    // A decoder that reads a Java value rather than JSON, as a JDBC column is read.
    static Decoder<Object, ?> objectDecoder(String name) {
        return switch (name) {
            // A Java byte is signed; the bytes are written as the unsigned numbers the case lists.
            case "bytes" -> ObjectDecoders.bytes().map(b -> {
                var numbers = new ArrayList<Integer>();
                for (byte v : b) numbers.add(v & 0xff);
                return numbers;
            });
            default -> throw new IllegalArgumentException("no decoder " + name);
        };
    }

    static Decoder<JsonNode, List<Object>> contactDecoder() {
        return combine(field("email", string()), field("phone", string()))
                .map((e, p) -> List.of(e, p));
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
        // Read with each number as written, as DecodeJSON reads it: a mapper's tree would have
        // made a double of every number with a fraction or an exponent.
        var cases = readTree(Files.readString(Path.of(args[0])));
        var results = new ArrayList<Map<String, Object>>();
        for (var c : cases) {
            var name = c.get("decoder").asString();
            // A number whose text matters, such as -0, is given as the JSON text to read.
            var text = c.get("input_json");
            var bytes = c.get("input_bytes");
            var input = text != null ? readTree(text.asString()) : c.get("input");
            var result = new LinkedHashMap<String, Object>();
            result.put("decoder", name);
            if (bytes != null) {
                result.put("input_bytes", bytes);
            } else if (text != null) {
                result.put("input_json", text.asString());
            } else {
                result.put("input", input);
            }
            Result<?> r;
            if (bytes != null) {
                // Bytes are not JSON: null is null, and a list of unsigned numbers is a byte[].
                Object in = null;
                if (!bytes.isNull()) {
                    var array = new byte[bytes.size()];
                    for (int n = 0; n < array.length; n++) array[n] = (byte) bytes.get(n).asInt();
                    in = array;
                }
                r = objectDecoder(name).decode(in);
            } else {
                r = decoder(name).decode(input);
            }
            switch (r) {
                case Ok<?> ok -> result.put("ok", ok.value());
                case Err<?> err -> {
                    var issues = new ArrayList<Map<String, Object>>();
                    // Go gives the message its catalogue resolves, so the Java one is resolved too:
                    // the default a decoder writes says "entries" where the catalogue says "elements".
                    for (Issue issue : err.issues().resolve(MessageResolver.DEFAULT).asList()) {
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
