# node.context

Profile: env

File: Fixture Design System (FixTuReDeSiGnSySt3m01), version 2100123456

## Node: Login (2:2, FRAME)

- page: Screens
- path: Screens / Onboarding / Login
- size: 390 x 844

### Layout

#### 2:2 Login (FRAME)

- box: 390 x 844 at 0, 0 (relative 40, 80)
- layout: column; gap 16px [space/4]; padding 24px; justify flex-start; align flex-start; w fixed 390px; h fixed 844px; absolute at 40, 80 (left/top); clips
- fills: #ffffff [bg/surface]
- effects: box-shadow: 0px 4px 12px rgba(0, 0, 0, 0.1) {shadow/md}
- tokens: fills[0]=bg/surface, gap=space/4
- styles: effect=shadow/md, grid=grid/12

```css
align-items: flex-start;
background: var(--bg-surface);
box-shadow: 0px 4px 12px rgba(0, 0, 0, 0.1);
display: flex;
flex-direction: column;
gap: var(--space-4);
height: 844px;
justify-content: flex-start;
left: 40px;
overflow: hidden;
padding: 24px;
position: absolute;
top: 80px;
width: 390px;
```

##### 2:3 Heading (TEXT)

- box: 342 x 34 at 24, 24 (relative 24, 24)
- layout: w fill 342px; h hug 34px
- fills: #111827 [text/primary]
- text: "Welcome back"
- font: Inter [font/family/sans] 700 28px/34px tracking -0.5px {heading/lg}
- tokens: fills[0]=text/primary, fontFamily=font/family/sans
- styles: text=heading/lg

```css
align-self: stretch;
color: var(--text-primary);
font-family: var(--font-sans);
font-size: 28px;
font-weight: 700;
letter-spacing: -0.5px;
line-height: 34px;
```

##### 2:4 Body (TEXT)

- box: 342 x 24 at 24, 74 (relative 24, 74)
- layout: w fill 342px; h hug 24px
- fills: #6b7280
- text: "Sign in to continue to your account."
- font: Inter 400 16px/24px
- run 11-19 "continue": 600 link https://example.com/help color #3366ff [brand/500]
- tokens: runs[0].fill=brand/500

```css
align-self: stretch;
color: #6b7280;
font-family: Inter;
font-size: 16px;
font-weight: 400;
line-height: 1.5;
```

##### 2:5 Input/Text (INSTANCE)

- box: 342 x 56 at 24, 114 (relative 24, 114)
- layout: column; gap 4px; justify flex-start; align flex-start; w fill 342px; h hug 56px
- component: instance of Input/Text; props Label=Email, Show helper=false; overrides characters on 1 layer

```css
align-items: flex-start;
align-self: stretch;
display: flex;
flex-direction: column;
gap: 4px;
justify-content: flex-start;
```

###### I2:5;3:21 Label (TEXT)

- box: 342 x 12 at 24, 114 (relative 0, 0)
- layout: w fill 342px; h hug 12px
- fills: #111827
- text: "Email"
- font: Inter 500 12px/12px

```css
align-self: stretch;
color: #111827;
font-family: Inter;
font-size: 12px;
font-weight: 500;
line-height: normal;
```

###### I2:5;3:22 Field (FRAME)

- box: 342 x 40 at 24, 130 (relative 0, 16)
- layout: row; gap 8px; padding 0px 12px; justify flex-start; align center; w fill 342px; h fixed 40px; clips
- fills: #ffffff
- strokes: #6b7280 1px inside
- radius: 6px

```css
align-items: center;
align-self: stretch;
background: #ffffff;
border: 1px solid #6b7280;
border-radius: 6px;
box-sizing: border-box;
display: flex;
flex-direction: row;
gap: 8px;
height: 40px;
justify-content: flex-start;
overflow: hidden;
padding: 0px 12px;
```
- children: 1 (not expanded)

##### 2:6 Button/Primary (INSTANCE)

- box: 342 x 48 at 24, 186 (relative 24, 186)
- layout: row; gap 8px; padding 12px 20px; justify center; align center; w fill 342px; h fixed 48px; clips
- fills: #3366ff [brand/500] {color/brand/500}
- radius: 8px
- component: instance of Button; variant Size=md, Variant=Primary; props Label=Sign in; overrides characters, layoutAlign, layoutSizingHorizontal on 2 layers
- interaction: ON_CLICK -> NODE 2:14 (SMART_ANIMATE 0.3s)
- tokens: fills[0]=brand/500
- styles: fill=color/brand/500

```css
align-items: center;
align-self: stretch;
background: var(--color-brand-500);
border-radius: 8px;
display: flex;
flex-direction: row;
gap: 8px;
height: 48px;
justify-content: center;
overflow: hidden;
padding: 12px 20px;
```

###### I2:6;3:12 Label (TEXT)

- box: 52 x 24 at 169, 198 (relative 145, 12)
- layout: w hug 52px; h hug 24px
- fills: #ffffff [neutral/0]
- text: "Sign in"
- font: Inter 600 16px/24px align center
- tokens: fills[0]=neutral/0

```css
color: #ffffff;
font-family: Inter;
font-size: 16px;
font-weight: 600;
line-height: 1.5;
text-align: center;
```

##### 2:7 Social (FRAME)

- box: 342 x 40 at 24, 250 (relative 24, 250)
- layout: wrap; gap 12px row-gap 8px; justify flex-start; align center; w fill 342px; h hug 40px

```css
align-items: center;
align-self: stretch;
column-gap: 12px;
display: flex;
flex-direction: row;
flex-wrap: wrap;
justify-content: flex-start;
row-gap: 8px;
```

###### 2:8 Avatar (RECTANGLE)

- box: 40 x 40 at 24, 250 (relative 0, 0)
- layout: w fixed 40px; h fixed 40px
- fills: image img1 fill
- radius: 20px

```css
background-image: url(img1);
background-size: cover;
border-radius: 20px;
height: 40px;
width: 40px;
```

###### 2:9 icon/arrow-right (VECTOR)

- box: 24 x 24 at 76, 258 (relative 52, 8)
- layout: w fixed 24px; h fixed 24px
- fills: #111827

```css
background: #111827;
height: 24px;
width: 24px;
```

##### 2:10 Decor (GROUP)

- box: 342 x 120 at 24, 306 (relative 24, 306)
- layout: w fixed 342px; h fixed 120px
- opacity: 0.9

```css
height: 120px;
opacity: 0.9;
width: 342px;
```

###### 2:11 Blob (ELLIPSE)

- box: 120 x 120 at 24, 306 (relative 0, 0)
- layout: w fixed 120px; h fixed 120px; absolute at 0, 0 (scale/scale)
- fills: linear-gradient(90deg, #3366ff 0%, #9933ff 100%)
- effects: filter: blur(1px)
- tokens: fills[0].stops[0]=brand/500

```css
background-image: linear-gradient(90deg, #3366ff 0%, #9933ff 100%);
filter: blur(1px);
height: 120px;
left: 0px;
position: absolute;
top: 0px;
width: 120px;
```

###### 2:12 Divider (LINE)

- box: 206 x 0 at 160, 366 (relative 136, 60)
- layout: w fixed 206px; h fixed 0px; absolute at 136, 60 (scale/scale)
- strokes: #f3f4f6 1px center dashed

```css
border: 1px dashed #f3f4f6;
height: 0px;
left: 136px;
position: absolute;
top: 60px;
width: 206px;
```

##### 2:14 Stats (FRAME)

- box: 342 x 120 at 24, 498 (relative 24, 498)
- layout: grid; 2x2; gap 8px 8px; w fill 342px; h fixed 120px

```css
align-self: stretch;
column-gap: 8px;
display: grid;
grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
grid-template-rows: minmax(0, 1fr) minmax(0, 1fr);
height: 120px;
row-gap: 8px;
```

###### 2:15 Stat (FRAME)

- box: 167 x 120 at 24, 498 (relative 0, 0)
- layout: w fill 167px; h fill 120px; cell 1,1 span 2x1
- fills: #f3f4f6
- radius: 8px

```css
background: #f3f4f6;
border-radius: 8px;
grid-column: 1 / span 1;
grid-row: 1 / span 2;
```

###### 2:16 Count (TEXT)

- box: 167 x 56 at 199, 498 (relative 175, 0)
- layout: w hug 167px; h hug 56px; cell 1,2 span 1x1
- fills: #111827
- text: "128"
- font: Inter 700 24px/28px uppercase align center

```css
align-self: center;
color: #111827;
font-family: Inter;
font-size: 24px;
font-variant-numeric: tabular-nums;
font-weight: 700;
grid-column: 2 / span 1;
grid-row: 1 / span 1;
justify-self: center;
line-height: 28px;
text-align: center;
text-transform: uppercase;
```

###### 2:17 Caption (TEXT)

- box: 167 x 56 at 199, 562 (relative 175, 64)
- layout: w hug 167px; h hug 56px; cell 2,2 span 1x1
- fills: #6b7280
- text: "Designs shipped"
- font: Inter 400 12px/16px tracking 0.2px align center truncate 1 lines

```css
align-self: start;
color: #6b7280;
font-family: Inter;
font-size: 12px;
font-weight: 400;
grid-column: 2 / span 1;
grid-row: 2 / span 1;
justify-self: center;
letter-spacing: 0.2px;
line-height: 16px;
overflow: hidden;
text-align: center;
text-overflow: ellipsis;
white-space: nowrap;
```

## Tokens used

| token | collection | type | values | code |
| --- | --- | --- | --- | --- |
| bg/surface | Semantic | COLOR | Dark=#111827, Light=#ffffff | WEB: --bg-surface |
| brand/500 | Primitives | COLOR | Default=#3366ff | WEB: --color-brand-500 |
| font/family/sans | Primitives | STRING | Default=Inter | WEB: --font-sans |
| neutral/0 | Primitives | COLOR | Default=#ffffff |  |
| space/4 | Primitives | FLOAT | Default=16 | WEB: --space-4 |
| text/primary | Semantic | COLOR | Dark=#ffffff, Light=#111827 | WEB: --text-primary |

| style | type | nodeId |
| --- | --- | --- |
| color/brand/500 | FILL | 5:1 |
| grid/12 | GRID | 5:4 |
| heading/lg | TEXT | 5:2 |
| shadow/md | EFFECT | 5:3 |

## Components

### Input/Text

- key: a1b2c3d4e5f60718293a4b5c6d7e8f9012345320
- properties: Label#12:0 (TEXT, default Label); Show helper#12:1 (BOOLEAN, default false)
- description: Single line text input with a label and optional helper text.
- docs: https://example.com/design-system/input
- code: Input.tsx on GitHub https://github.com/example/design-system/blob/main/src/Input.tsx
- instances: 2:5

### Button (Variant=Primary, Size=md)

- set: Button (3:10)
- key: a1b2c3d4e5f60718293a4b5c6d7e8f9012345311
- variant: Size=md, Variant=Primary
- properties: Label#5:0 (TEXT, default Button); Size (VARIANT: sm|md, default md); Variant (VARIANT: Primary|Secondary, default Primary)
- description: Primary call to action.
- docs: https://example.com/design-system/button
- code: Button in Storybook https://storybook.example.com/?path=/story/button--primary
- instances: 2:6

## Assets

| kind | id | name | path | error |
| --- | --- | --- | --- | --- |
| icon | 2:9 | icon/arrow-right | <OUT>/icon-arrow-right.svg |  |
| imageFill | img1 | Avatar (2:8) | <OUT>/imageref-img1.png |  |

## Screenshot path

| nodeId | name | path | scale |
| --- | --- | --- | --- |
| 2:2 | Login | <OUT>/login@2x.png | 2x |

Read these files with your image or vision tool before writing code.

## Measurements

Distances the designer pinned in Dev Mode. Prefer these over a gap read off the screenshot.

| value | axis | from | to | note |
| --- | --- | --- | --- | --- |
| 56px | vertical | Heading bottom | Input/Text top |  |
| 16-24 responsive (set by the designer) | vertical | Input/Text bottom | Button/Primary top |  |

## Comments

- Fixture Designer on 2:2 (2026-08-28T14:02:00Z, open): Use the **primary** button here; the label should read "Sign in".
  - reply Fixture Engineer (2026-08-28T16:30:00Z, open): Done, using Button/Primary.


Hints:
- Read the screenshot with your image or vision tool before writing code: <OUT>/login@2x.png
- Every value with "token": null is not bound to a variable: hardcode it, or ask the designer for a token. Values with a token name should use that design token in code.
- Assets were written to <OUT>: inline or import the SVG icons and reference the image fills by path.
- The designer pinned 2 measurement(s) in Dev Mode; see data.measurements. They state a spacing decision outright, so prefer them over a gap read off the screenshot.
- When the implementation is ready, run figctl render FixTuReDeSiGnSySt3m01 --node 2:2 and compare it with your result.
