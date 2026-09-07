# Fixture Design System

Synthetic Figma file used by every test. All files agree on the same IDs, so
a node ID, style ID, variable ID, component key, or file key found in one file
can be looked up in the others.

| Identifier | Value |
| --- | --- |
| File key | `FixTuReDeSiGnSySt3m01` |
| Branch key | `BrAnChKeY0000000000001` (`feature/dark-mode`) |
| File version | `2100123456` |
| Last modified / last touched | `2026-09-01T10:15:00Z` |
| Token accepted by the fake server | `figd_fixture_token` |
| Team, project, folder IDs | `555000111`, `1001`, `2001` |
| User | id `1234567890`, handle `Fixture Designer`, `designer@example.com` |

## Document tree (`file.json`)

```
0:0 DOCUMENT "Document"
├── 0:1 CANVAS "Screens"                       flow starting point on 2:2
│   └── 2:1 SECTION "Onboarding"               devStatus READY_FOR_DEV
│       └── 2:2 FRAME "Login" 390x844          VERTICAL auto layout, padding 24, gap 16
│           │                                  fills bound to bg/surface (1:201), itemSpacing bound to space/4 (1:104)
│           │                                  effect style 5:3 (shadow/md), grid style 5:4 (grid/12), PNG export @2x
│           ├── 2:3 TEXT "Heading"             "Welcome back", Inter 700 28/34, text style 5:2 (heading/lg)
│           │                                  fill bound to text/primary (1:202), fontFamily bound to font/family/sans (1:105)
│           ├── 2:4 TEXT "Body"                "Sign in to continue to your account."; run 1 ("continue") is SemiBold,
│           │                                  brand colored (bound to 1:101) and hyperlinked
│           ├── 2:5 INSTANCE "Input/Text"      of 3:20; props Label#12:0="Email" (TEXT), Show helper#12:1=false (BOOLEAN)
│           │   ├── I2:5;3:21 TEXT "Label"     "Email" (overridden characters)
│           │   └── I2:5;3:22 FRAME "Field"    stroke neutral/500, radius 6
│           │       └── I2:5;3:23 TEXT "Placeholder"
│           ├── 2:6 INSTANCE "Button/Primary"  of 3:11; props Variant=Primary, Size=md, Label#5:0="Sign in"
│           │   │                              overrides on 2:6 (sizing) and I2:6;3:12 (characters)
│           │   │                              fill style 5:1 + bound brand/500; ON_CLICK navigates to 2:14
│           │   └── I2:6;3:12 TEXT "Label"     "Sign in", fill bound to neutral/0 (1:103)
│           ├── 2:7 FRAME "Social"             HORIZONTAL auto layout with WRAP, gap 12, row gap 8
│           │   ├── 2:8 RECTANGLE "Avatar"     IMAGE fill imageRef "img1", radius 20, smoothing 0.6
│           │   └── 2:9 VECTOR "icon/arrow-right"  fillGeometry path, exportSettings SVG and PNG@2x
│           ├── 2:10 GROUP "Decor"             opacity 0.9
│           │   ├── 2:11 ELLIPSE "Blob"        GRADIENT_LINEAR fill (stop 0 bound to brand/500), LAYER_BLUR effect
│           │   └── 2:12 LINE "Divider"        dashed stroke [4,4]
│           ├── 2:13 FRAME "Debug"             visible: false (renders as null in images)
│           └── 2:14 FRAME "Stats"             GRID layout 2x2, row/column gap 8
│               ├── 2:15 FRAME "Stat"          gridRowSpan 2
│               ├── 2:16 TEXT "Count"          "128", UPPER case, tabular numerals
│               └── 2:17 TEXT "Caption"        "Designs shipped", truncation ENDING, maxLines 1
└── 1:2 CANVAS "Design System"
    ├── 3:10 COMPONENT_SET "Button"            props Variant: Primary|Secondary, Size: sm|md, Label#5:0 (TEXT)
    │   ├── 3:11 COMPONENT "Variant=Primary, Size=md"    160x48, fill style 5:1, child 3:12 TEXT "Label"
    │   ├── 3:13 COMPONENT "Variant=Secondary, Size=md"  child 3:14
    │   ├── 3:15 COMPONENT "Variant=Primary, Size=sm"    120x36, child 3:16
    │   └── 3:17 COMPONENT "Variant=Secondary, Size=sm"  child 3:18
    ├── 3:20 COMPONENT "Input/Text"            props Label#12:0 (TEXT "Label"), Show helper#12:1 (BOOLEAN false)
    │   ├── 3:21 TEXT "Label"                  characters bound to Label#12:0
    │   ├── 3:22 FRAME "Field"
    │   │   └── 3:23 TEXT "Placeholder"
    │   └── 3:24 TEXT "Helper"                 visible false, visibility bound to Show helper#12:1
    └── 4:0 FRAME "Styles"                     sample nodes that use each style
        ├── 4:1 RECTANGLE "color/brand/500"    styles.fill = 5:1
        ├── 4:2 TEXT "heading/lg"              styles.text = 5:2
        ├── 4:3 RECTANGLE "shadow/md"          styles.effect = 5:3
        └── 4:4 FRAME "grid/12"                styles.grid = 5:4
```

Top level maps in `file.json`:

- `components`: `3:11`, `3:13`, `3:15`, `3:17` (set `3:10`) and `3:20`; keys `a1b2...53xx` where `xx` matches the node id.
- `componentSets`: `3:10` Button, key `a1b2...5310`.
- `styles`: `5:1` FILL `color/brand/500`, `5:2` TEXT `heading/lg`, `5:3` EFFECT `shadow/md`, `5:4` GRID `grid/12`; keys `5f4e...d50x`.
- `branches`: one branch, returned only with `branch_data=true`.

## Style nodes (`nodes.json`)

Style values are not part of the document tree, exactly as in the real API.
`nodes.json` holds the four style nodes `5:1` to `5:4` in `GET nodes` shape.
The fake server serves them for `GET /v1/files/:key/nodes?ids=5:1,...` and
serves every document node from `file.json` for any other id, wrapping each
with the components, component sets, and styles its subtree references.

## Variables (`variables_local.json`, `variables_published.json`)

| Collection | Modes | Variables |
| --- | --- | --- |
| `VariableCollectionId:1:100` Primitives | `1:0` Default | `1:101` brand/500 COLOR #3366FF (`--color-brand-500`), `1:102` neutral/900 COLOR #111827, `1:103` neutral/0 COLOR #FFFFFF, `1:104` space/4 FLOAT 16 (`--space-4`, scopes GAP, WIDTH_HEIGHT), `1:105` font/family/sans STRING "Inter" (`--font-sans`), `1:106` feature/show-helper BOOLEAN true (hidden from publishing) |
| `VariableCollectionId:1:200` Semantic | `2:0` Light, `2:1` Dark | `1:201` bg/surface COLOR (Light: alias neutral/0, Dark: alias neutral/900), `1:202` text/primary COLOR (Light: alias neutral/900, Dark: alias neutral/0) |

Variable IDs are written `VariableID:1:NNN`. The published file omits
`1:106` (hidden) and carries `subscribed_id` values of the form `<id>/1`.

## Other fixtures

- `meta.json`: file meta with `last_touched_at` and `version` equal to the file.
- `styles.json`, `components.json`, `component_sets.json`: library list responses; `node_id` values match the document.
- `images.json`: render URLs for `2:2` and `2:9`, `null` for hidden `2:13`. The server generates `<server>/images/<id>.<format>` for any visible node.
- `image_fills.json`: `img1` download URL. The server rewrites the CDN host to its own address and serves bytes under `/images/` and `/img/`.
- `comments.json`: `9001` pinned on node `2:2` with a reaction, `9002` reply to `9001`, `9003` resolved comment pinned on the canvas.
- `versions.json`: `2100123456` (labelled "Login v2") and `2100120000`, with a `next_page` link using `before=`.
- `dev_resources.json`: `dr-1` on `3:11` (Storybook), `dr-2` on `3:20` (GitHub).
- `me.json`, `projects.json`, `project_files.json`, `folders.json`, `folder_folders.json`, `folder_files.json`, `folder_meta.json`.

## Fake server behavior

- Requires `X-Figma-Token: figd_fixture_token`; otherwise `403 {"status":403,"err":"Invalid token"}`.
- Unknown file keys, teams, projects, and folders return 404 in Figma's shape.
- `GET file` honors `depth` and `branch_data`; `GET nodes` honors `ids` and `depth`.
- `GET images` returns `null` for hidden nodes and 400 for unknown ids.
- Team library endpoints honor `page_size` and `after`; versions honor `page_size` and `before`.
- `Enqueue` injects one-shot responses (429, 500, 401, 403) ahead of the default handling; `Respond` and `Handle` override a path permanently; `Count` and `Requests` inspect traffic.
