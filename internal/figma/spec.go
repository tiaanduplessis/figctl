package figma

// The response types, endpoints, and node properties in this package are
// hand-written rather than generated from Figma's OpenAPI document: the
// available Go generators handle that OpenAPI 3.1 spec poorly and produce
// a model that is worse to work with than the subset written by hand.
// The cost of that choice is that nothing notices when Figma changes the
// API, so the spec version is pinned here instead and checked against the
// published one by "make spec-check".

// SpecVersion is the info.version of the Figma OpenAPI specification that
// the types in this package were written against. Bump it only together
// with a review of what changed in the spec.
//
// When "make spec-check" reports a different version: read the changelog,
// diff the new spec against the endpoints and node properties declared
// here, apply the changes by hand, then move this constant. The
// "Tracking the Figma API" section of CONTRIBUTING.md describes the
// review.
const SpecVersion = "0.42.0"

// SpecURL is the raw OpenAPI document that "make spec-check" downloads.
const SpecURL = "https://raw.githubusercontent.com/figma/rest-api-spec/main/openapi/openapi.yaml"

// SpecChangelogURL is Figma's REST API changelog, which explains what
// changed between two spec versions.
const SpecChangelogURL = "https://www.figma.com/developers/api#changelog"

// SpecReleasesURL lists the tagged releases of the spec repository, which
// is the quickest way to see the diff between two versions.
const SpecReleasesURL = "https://github.com/figma/rest-api-spec/releases"
