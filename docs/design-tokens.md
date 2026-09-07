# Design tokens

`figctl tokens export` turns the variables and styles of a Figma file into
design tokens. The default format is the W3C Design Tokens Format Module
2025.10, the first stable version of the DTCG spec, released in October 2025.
Style Dictionary v4 and later consume it unchanged.

```sh
figctl tokens export KEY > tokens.json                       # DTCG
figctl tokens export KEY --format css --out ./src/styles     # custom properties
figctl tokens export KEY --format tailwind                   # theme module
figctl tokens export KEY --format style-dictionary           # identical to DTCG
figctl tokens export KEY --format json                       # figctl's flat list with Figma ids
```

Without variables access, which means any plan below Enterprise or a token
without `file_variables:read`, the command still exports the styles and says so
in `hints`. It does not fail.

## Naming

Figma names group on `/`, so `color/brand/500` becomes the DTCG path
`color.brand.500`. Segments are normalized by `--name-case`, which accepts
`kebab` (the default), `camel`, `snake`, or `none`. `none` keeps the Figma
segment as written, minus the characters DTCG forbids in a name: `{`, `}`, `.`,
`$`, and control characters.

If two Figma names normalize to the same path the export fails with `USAGE`
rather than silently dropping one, and the hint offers `--name-case none` or
`--collection` to separate them.

## The DTCG mapping

### Variables

| Figma resolved type | `$type` | `$value` |
| --- | --- | --- |
| `COLOR` | `color` | hex string, `#rrggbb` or `#rrggbbaa` |
| `FLOAT` with a length scope | `dimension` | `{"value": 16, "unit": "px"}` |
| `FLOAT` otherwise | `number` | the number |
| `STRING` scoped to `FONT_FAMILY` | `fontFamily` | the family name |
| `STRING` otherwise | `string` | the string |
| `BOOLEAN` | `boolean` | the boolean |

The length scopes that make a `FLOAT` a `dimension` are `WIDTH_HEIGHT`, `GAP`,
`CORNER_RADIUS`, `STROKE_FLOAT`, `PARAGRAPH_SPACING`, and `PARAGRAPH_INDENT`.

Colors are hex strings rather than the 2025.10 color object, because a Figma
color is sRGB and the object form carries no extra information for it.

`boolean` is not a DTCG type. A Figma boolean variable is emitted as
`"$type": "boolean"` anyway, because dropping it would lose a real value and
consumers ignore types they do not know.

A variable alias becomes a DTCG reference, `{group.token}`, when the target is
in the same document. Aliases that cross a document boundary are resolved to
their concrete value.

Figma information that DTCG has no field for is kept under
`$extensions."com.figma"`: `scopes`, `codeSyntax`, and, under the default mode
strategy, `modes`.

### Styles

Figma styles are richer than DTCG in several places. Each case is handled
explicitly and described in the token's `$description`, never dropped silently.

| Figma style | Result |
| --- | --- |
| `FILL`, single solid paint | `color` |
| `FILL`, linear gradient | `gradient`. The DTCG gradient type carries stops only, so the angle and the CSS declaration go under `$extensions."com.figma".gradient` |
| `FILL`, radial, angular, or diamond gradient | not representable: these are approximations even in CSS, and a flat colour would change the design, so the token is a `string` holding the CSS gradient, with a `$description` saying so |
| `FILL`, image or pattern paint | `string` holding the Figma image reference, which is all the REST API exposes |
| `TEXT` | `typography` composite |
| `EFFECT` with shadows | `shadow`, an array when there are several |
| `EFFECT`, blur only | blur has no DTCG type, so the token is a `string` holding the CSS `filter` value, with a `$description` |
| `EFFECT`, blurs alongside shadows | the shadows become `shadow`; the blurs are kept under `$extensions."com.figma".filters` |
| `GRID` | DTCG has no layout grid type at all. Rather than dropping layout grids the token is a `string` summary with the full definition under `$extensions."com.figma".grid`, so a build step can still read the column count, gutter, and margin |

## Modes

A Figma collection can have several modes: light and dark, breakpoints, brands,
density. A DTCG document has one value per token, so a collection with more than
one mode cannot be flattened without a decision. `--mode-strategy` makes it
explicit.

| Strategy | Behaviour |
| --- | --- |
| `default` | emit the default mode of every collection and record the other modes under `$extensions."com.figma".modes` |
| `separate` | emit one document per mode name (`tokens.light.json`, `tokens.dark.json`) |
| `select` | emit one document using the modes named by `--mode` |

`--mode Dark` implies `--mode-strategy select` on its own. `--mode` is
repeatable, once per collection.

```sh
figctl tokens export KEY --mode-strategy separate --out ./tokens
figctl tokens export KEY --mode Dark --mode Compact
```

## CSS output

`--format css` writes custom properties: a `:root` block for the selected modes
and one block per remaining mode.

```css
/* Design tokens from Fixture Design System (version 2100123456), generated by figctl. Do not edit by hand. */

:root {
  --color-brand-500: #3366ff;
  --font-sans: Inter;
  --neutral-0: #ffffff;
  --neutral-900: #111827;
  --space-4: 16px;
  --bg-surface: var(--neutral-0);
  --text-primary: var(--neutral-900);
  --shadow-md: 0px 4px 12px rgba(0, 0, 0, 0.1);
  --grid-12-columns: 12;
  --grid-12-gutter: 16px;
  --grid-12-margin: 24px;
  --heading-lg-font-family: Inter;
  --heading-lg-font-size: 28px;
  --heading-lg-font-weight: 700;
  --heading-lg-line-height: 1.21;
  --heading-lg-letter-spacing: -0.5px;
}

[data-theme="dark"] {
  --bg-surface: #111827;
  --text-primary: #ffffff;
}
```

Details worth knowing:

- A property is named after the variable's `WEB` `codeSyntax` when it has one,
  because that field exists precisely to say what the variable is called in
  code. Otherwise the name is the kebab-cased token path.
- Aliases stay as `var(--target)` in `:root`. Mode blocks inline their values,
  because the alias target of another mode is not part of the resolved document.
- Typography has no composite form in CSS, so a text style expands into one
  property per field. Layout grids expand into column count, gutter, and margin.
- CSS expresses modes with selectors, so every mode is always written whatever
  `--mode-strategy` asked for. The strategy only decides which mode fills
  `:root`.
- `--mode-selector` changes the per-mode selector template. The default is
  `[data-theme="<mode>"]`, where `<mode>` is replaced with the kebab-cased mode
  name.
- If two tokens claim the same custom property with different values, the first
  wins and the conflict is reported in `warnings`.

## Tailwind output

`--format tailwind` writes a theme module, as ESM by default,
`--tailwind-format cjs|json` for the alternatives. `--tailwind-vars` emits
`var(--name)` references instead of literals, for projects that ship the CSS
output too.

Tokens are placed under the theme key their type and Figma scopes imply:
`colors`, `spacing`, `fontFamily`, `fontSize`, `fontWeight`, `borderRadius`,
`boxShadow`. A leading group segment that repeats the theme key is dropped, so
`color/brand/500` becomes `colors.brand.500` rather than
`colors.color.brand.500`. Every value is a string, because that is what
Tailwind's theme expects.

Tailwind's theme has no key for typography composites, gradients, grids,
booleans, or plain numbers, so those go under `extend`, grouped by token type,
with a comment explaining why. Key collisions are reported in `warnings`.

## Filtering

| Flag | Effect |
| --- | --- |
| `--include variables\|styles` | choose the sources; both by default |
| `--collection NAME` | only this collection, repeatable |
| `--exclude-hidden` | drop variables marked `hiddenFromPublishing` |
| `--exclude-remote` | drop variables and styles from remote libraries |

## Resolving a single token

`figctl tokens resolve` answers "what is this actually worth" for one variable or
style, including the alias chain per mode:

```sh
figctl tokens resolve KEY --name bg/surface
```

```json
{
  "kind": "variable",
  "variable": {
    "id": "VariableID:1:201",
    "name": "bg/surface",
    "collection": "Semantic",
    "type": "COLOR",
    "values": {"Light": "#ffffff", "Dark": "#111827"},
    "codeSyntax": {"WEB": "--bg-surface"},
    "modes": [
      {"mode": "Light", "modeId": "2:0", "default": true, "value": "#ffffff", "chain": ["bg/surface", "neutral/0"]},
      {"mode": "Dark", "modeId": "2:1", "value": "#111827", "chain": ["bg/surface", "neutral/900"]}
    ]
  }
}
```
