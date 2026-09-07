# node.inspect

Profile: env

File: Fixture Design System (FixTuReDeSiGnSySt3m01), version 2100123456

## 2:2 Login (FRAME)

- box: 390 x 844 at 0, 0 (relative 40, 80)
- layout: column; gap 16px [space/4]; padding 24px; justify flex-start; align flex-start; w fixed 390px; h fixed 844px; absolute at 40, 80 (left/top); clips
- fills: #ffffff [bg/surface]
- effects: box-shadow: 0px 4px 12px rgba(0, 0, 0, 0.1) {shadow/md}
- tokens: fills[0]=bg/surface, gap=space/4
- styles: effect=shadow/md, grid=grid/12

### 2:3 Heading (TEXT)

- box: 342 x 34 at 24, 24 (relative 24, 24)
- layout: w fill 342px; h hug 34px
- fills: #111827 [text/primary]
- text: "Welcome back"
- font: Inter [font/family/sans] 700 28px/34px tracking -0.5px {heading/lg}
- tokens: fills[0]=text/primary, fontFamily=font/family/sans
- styles: text=heading/lg

### 2:4 Body (TEXT)

- box: 342 x 24 at 24, 74 (relative 24, 74)
- layout: w fill 342px; h hug 24px
- fills: #6b7280
- text: "Sign in to continue to your account."
- font: Inter 400 16px/24px
- run 11-19 "continue": 600 link https://example.com/help color #3366ff [brand/500]
- tokens: runs[0].fill=brand/500

### 2:5 Input/Text (INSTANCE)

- box: 342 x 56 at 24, 114 (relative 24, 114)
- layout: column; gap 4px; justify flex-start; align flex-start; w fill 342px; h hug 56px
- component: instance of Input/Text; props Label=Email, Show helper=false; overrides characters on 1 layer
- children: 2 (not expanded)

### 2:6 Button/Primary (INSTANCE)

- box: 342 x 48 at 24, 186 (relative 24, 186)
- layout: row; gap 8px; padding 12px 20px; justify center; align center; w fill 342px; h fixed 48px; clips
- fills: #3366ff [brand/500] {color/brand/500}
- radius: 8px
- component: instance of Button; variant Size=md, Variant=Primary; props Label=Sign in; overrides characters, layoutAlign, layoutSizingHorizontal on 2 layers
- tokens: fills[0]=brand/500
- styles: fill=color/brand/500
- children: 1 (not expanded)

### 2:7 Social (FRAME)

- box: 342 x 40 at 24, 250 (relative 24, 250)
- layout: wrap; gap 12px row-gap 8px; justify flex-start; align center; w fill 342px; h hug 40px
- children: 2 (not expanded)

### 2:10 Decor (GROUP)

- box: 342 x 120 at 24, 306 (relative 24, 306)
- layout: w fixed 342px; h fixed 120px
- opacity: 0.9
- children: 2 (not expanded)

### 2:14 Stats (FRAME)

- box: 342 x 120 at 24, 498 (relative 24, 498)
- layout: grid; 2x2; gap 8px 8px; w fill 342px; h fixed 120px
- children: 3 (not expanded)

