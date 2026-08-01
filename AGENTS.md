# AGENTS.md

## Project context

Treat repositories beneath the project root as components of one project. Use the external Obsidian folder for durable knowledge and work tracking.

```yaml
project: "Herb Hub"
project_root: "/Users/andybarber/repos/Herb-Hub"
current_repository:
  name: "HerbHub365"
  path: "/Users/andybarber/repos/Herb-Hub/HerbHub365"
repository_discovery: project root and immediate child directories containing .git

knowledge:
  vault: "/Users/andybarber/Documents/obsidian_Vault/Herb-Hub"
  sources: "/Users/andybarber/Documents/obsidian_Vault/Herb-Hub/sources"
  wiki: "/Users/andybarber/Documents/obsidian_Vault/Herb-Hub/wiki"
  index: "/Users/andybarber/Documents/obsidian_Vault/Herb-Hub/wiki/index.md"
  home_note: "Herb Hub"

work_tracking:
  type: obsidian-kanban
  board: "/Users/andybarber/Documents/obsidian_Vault/Herb-Hub/kanban/Herb Hub Board.md"
  workflow: [Backlog, Ready, In Progress, Blocked, Done]
```

Verify paths and existing files before writing.

## Sources of truth

- **Repositories:** code, configuration, tests, schemas and deployment files.
- **Kanban board:** tasks, bugs, feature requests, priority and delivery status.
- **Sources:** read-only evidence such as research, logs, transcripts and imports.
- **Wiki:** concise, durable understanding derived from repositories and sources.

Link to source material instead of duplicating it.

## Retrieval

1. Read `wiki/index.md` first.
2. Read only relevant wiki pages.
3. Inspect repositories or `sources` only when verification or more detail is needed.
4. Read the Kanban board when planning work or checking task status.
5. Do not load the full vault or every repository without a specific reason.
6. State when evidence is incomplete, stale or contradictory.

## Work tracking

- Search the board before adding a card; update or move existing cards instead of duplicating them.
- Keep cards short. Put implementation detail in repositories and durable reasoning in the wiki.
- Reference repository content as `<repository>: <relative-path>` and link relevant `[[wiki notes]]`.
- Move a card to **In Progress** only when work begins.
- Move it to **Done** only after implementation and validation.
- Use **Blocked** when progress needs an external decision, dependency or access.
- If the vault is unavailable, report the proposed board change; never claim it was applied.

## Updating knowledge

Update the wiki only for durable knowledge: architecture, cross-repository relationships, significant decisions, reusable procedures, investigation findings, constraints and lessons.

Before writing:

1. Search the index and existing pages.
2. Prefer updating or merging over creating duplicates.
3. Create a page only for a distinct reusable subject.
4. Add source paths and useful `[[internal links]]`.
5. Update `wiki/index.md` when pages are added, renamed or materially changed.

Do not create wiki pages for routine commits, temporary tasks or every conversation.

## Wiki format

Use the relevant template from `wiki/templates`, keep pages concise and remove unused sections.

## Safety

- Never invent facts, commands, outputs, links or decisions.
- Never store credentials or secrets.
- Distinguish facts, assumptions, proposals and open questions.
- Treat `sources` as read-only unless explicitly instructed otherwise.
- Preserve useful content when editing wiki pages or the board.
- Do not delete, move, rename or bulk-reorganise vault files without approval.
- Supersede and link historical decisions rather than silently rewriting them.

## Output

When the vault is accessible, make requested changes and return a concise list of changed files and board movements. Otherwise provide the suggested file or board update without claiming it was saved.

## Principle

Compile evidence into a small connected wiki and track actionable work on the Kanban board. Read the index first and return to raw sources only when necessary.
