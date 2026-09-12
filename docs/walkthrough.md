# From a Figma frame to implementation context

This walkthrough uses a file you can already open in Figma. It reads the design
and writes local files; implementing the UI is the coding agent's next step.
Install figctl using the [quickstart](../README.md#quickstart).

## Authenticate and choose a frame

Create a personal access token with the required [read scopes](../README.md#tokens-and-scopes),
then paste it into the hidden prompt:

```sh
figctl auth login personal --default
figctl auth status
```

Check that the expected account is shown and `validated` is true. Copy your file
URL from Figma and use it in place of the example below:

```sh
figctl file tree "https://www.figma.com/design/KEY/App" --depth 2
```

The output lists node ids, types, and names. Choose a visible frame; replace
`KEY` with your file key and `2:2` with that frame's id in every following
command. You can also pass the frame's Figma URL, including its `node-id`.

## Save the context and reference image

```sh
figctl node context KEY --node 2:2 --json > context.json
figctl render KEY --node 2:2 --scale 1 --out ./.figctl/reference --json > render.json
```

`context.json` contains the normalized layout, styles, token references,
components, and paths to exported assets. `render.json` lists the reference PNG
path under `data.items`. Open that PNG and check it is the frame you selected.
Read `hints` in both files: optional data can require additional scopes or a
Figma plan. Exit 6 means partial success; inspect failures before continuing.
Keep the JSON files and `.figctl/` out of version control if the design is private.

## Give the context to your agent

Install the instructions for your agent from your project directory:

```sh
figctl skill install --agent codex --project
```

Other supported agents are listed in [Agent integration](agents.md). A sample
prompt is:

> Implement the frame described in context.json using this repository's existing
> components and styles. Open the reference image listed in render.json, reuse
> the exported assets, and check the result against that image. Read the hints
> before assuming optional design data is available.

Success means the agent can identify the chosen frame, inspect its layout and
styles, open its screenshot, and find its assets. UI correctness still needs
review in your application.
