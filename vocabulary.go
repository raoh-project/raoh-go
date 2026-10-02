package raoh

// Codes an [Issue] carries: what kind of problem it reports. They are shared
// with Raoh for Java, Rust and PHP, so a program that reads them reads the
// issues of any of them the same way.
const (
	CodeRequired         = "required"          // The value is missing or null.
	CodeBlank            = "blank"             // The string is empty or white space only.
	CodeTooShort         = "too_short"         // The string has fewer characters than allowed.
	CodeTooLong          = "too_long"          // The string has more characters than allowed.
	CodeInvalidLength    = "invalid_length"    // The string does not have exactly the required number of characters.
	CodeOutOfRange       = "out_of_range"      // The number is outside its bounds.
	CodeNotMultipleOf    = "not_multiple_of"   // The number is not a multiple of the divisor.
	CodeInvalidScale     = "invalid_scale"     // The decimal has more fraction digits than allowed.
	CodeTooSmall         = "too_small"         // The list has fewer elements than allowed.
	CodeTooBig           = "too_big"           // The list has more elements than allowed.
	CodeInvalidSize      = "invalid_size"      // The list does not have exactly the required number of elements.
	CodeInvalidValue     = "invalid_value"     // The value is not the one required.
	CodeInvalidFormat    = "invalid_format"    // The string does not have the required form.
	CodeTypeMismatch     = "type_mismatch"     // The value is of another type than the one required.
	CodeUnknownField     = "unknown_field"     // The object has a member no field declares.
	CodeMissingElement   = "missing_element"   // The list lacks a required element.
	CodeMissingElements  = "missing_elements"  // The list lacks some of the required elements.
	CodeDuplicateElement = "duplicate_element" // The list holds an element more than once.
	CodeNotAllowed       = "not_allowed"       // The value is not one of the allowed values.
	CodeOneOfFailed      = "one_of_failed"     // None of the alternatives decoded the value.
	CodeMissingField     = "missing_field"     // A member is missing.
)

// Message keys that name which check produced an issue, where one code covers
// several. A message key is its code followed by "." and a refinement, so a
// catalogue that has a template only for the code still resolves it. An issue
// whose check has no key of its own uses its code as its message key.
//
// These are the keys of the Raoh Specification 0.9.0, which Raoh for Java uses
// too. invalid_format.json is this package's own, for text that is not JSON,
// which the specification leaves outside its input model.
const (
	KeyOutOfRangeMinimum           = "out_of_range.minimum"
	KeyOutOfRangeMaximum           = "out_of_range.maximum"
	KeyOutOfRangeRange             = "out_of_range.range"
	KeyOutOfRangePositive          = "out_of_range.positive"
	KeyOutOfRangeNegative          = "out_of_range.negative"
	KeyOutOfRangeNonNegative       = "out_of_range.non_negative"
	KeyOutOfRangeNonPositive       = "out_of_range.non_positive"
	KeyOutOfRangeBefore            = "out_of_range.before"
	KeyOutOfRangeAfter             = "out_of_range.after"
	KeyOutOfRangeBetween           = "out_of_range.between"
	KeyTypeMismatchNumericRange    = "type_mismatch.numeric_range"
	KeyTooSmallNonEmpty            = "too_small.nonempty"
	KeyInvalidFormatEmail          = "invalid_format.email"
	KeyInvalidFormatURL            = "invalid_format.url"
	KeyInvalidFormatURI            = "invalid_format.uri"
	KeyInvalidFormatUUID           = "invalid_format.uuid"
	KeyInvalidFormatIP             = "invalid_format.ip"
	KeyInvalidFormatIPv4           = "invalid_format.ipv4"
	KeyInvalidFormatIPv6           = "invalid_format.ipv6"
	KeyInvalidFormatULID           = "invalid_format.ulid"
	KeyInvalidFormatCUID           = "invalid_format.cuid"
	KeyInvalidFormatStartsWith     = "invalid_format.starts_with"
	KeyInvalidFormatEndsWith       = "invalid_format.ends_with"
	KeyInvalidFormatIncludes       = "invalid_format.includes"
	KeyInvalidFormatEnum           = "invalid_format.enum"
	KeyInvalidFormatLiteral        = "invalid_format.literal"
	KeyInvalidFormatInstant        = "invalid_format.instant"
	KeyInvalidFormatDate           = "invalid_format.date"
	KeyInvalidFormatTime           = "invalid_format.time"
	KeyInvalidFormatDateTime       = "invalid_format.date_time"
	KeyInvalidFormatOffsetDateTime = "invalid_format.offset_date_time"
	KeyInvalidFormatJSON           = "invalid_format.json"
)
