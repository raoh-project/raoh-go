# Changelog

The major and minor version are the version of the Raoh Specification the module follows, and the
patch part is the module's own. A section's heading is the version alone; the release on GitHub
gives its date.

## 0.9.0

The first release. The decoders follow the Raoh Specification 0.9, and `scripts/conformance.sh`
checks them against every case of its suite: encode, messages-en and messages-ja are conformant,
and core is conformant but for one case declared a divergence, R001012. `ToSet` gives a Go map,
which holds `-0.0` and `0.0` of a list of floats as one element where the specification holds two.

The text rules come from notation-199x 0.2.0: White_Space, lengths in Unicode scalar values, case
conversion, normalization, the temporal grammar and the pattern language follow Unicode 18.0.0
whatever the Go release.

A module required at a commit before this release reads some input differently:

- `Pattern` takes the pattern language of the specification instead of the syntax of `regexp`.
- A string holding bytes that are not UTF-8 is `type_mismatch`.
- `Email` accepts the specification's ASCII profile of RFC 5321's `Mailbox`. A ULID takes either
  case and refuses a value past 128 bits. `URI` and `URL` accept the whole RFC 3986 production.
- Offset date-times compare by the instant. A float is rounded once, and `-0` stays -0.
- `Default` gives the default for a null or missing input, looked at before the decoder runs.
- Each field of an `Object` checks for itself that the input is an object, and a strict object
  nested in another reports an unknown member once.
- Floats in messages are written by Java's two-digit rule (`4.9E-324`).
- `invalid_format.json` is no longer in the shared message catalogue.

And in the API:

- `EnumOf` and `Literal` take `.Message`.
- `encode.Entry` owns one key, and `encode.Object` refuses a key given twice.
- `DiscriminateBy` is added.
