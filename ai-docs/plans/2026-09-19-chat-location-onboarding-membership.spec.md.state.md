# Interview state — chat location, deep-link onboarding, and player-to-chat membership

Handoff between rounds, and the re-entry point for every later return to `spec-writer`. Kept on `ready`.

```yaml
schema_version: 1
spec_path: ai-docs/plans/2026-09-19-chat-location-onboarding-membership.spec.md
issue_ref: "#30"
gh_issue:
  title: "Chat location, deep-link onboarding, and player-to-chat membership"
  state: open
  labels: ["mvp", "area:platform"]
  body: |
    ## What

    The first item of the MVP list: adding the bot to a chat creates that chat's location, and a player reaches the game through a deep link from a chat notification. This issue also settles the player-to-chat membership question, which several later mechanics hang off.

    ## Design refs

    - `docs/DESIGN.md` §14.1 — bot is added to a chat, a location is created.
    - `docs/DESIGN.md` §1 — all game commands happen in DM; the group chat receives only notifications. The chat is a collective entity: shared buildings, shared reputation, shared map knowledge.
    - `docs/DESIGN.md` §1 — **onboarding is embedded in notifications**: the bot cannot DM anyone who has not pressed Start, so a "join the raid" button in the chat asks the player to press Start, and then they are in the game.
    - `docs/DESIGN.md` §2.1 — the chat's home is a fully peaceful zone: no PvP, no looting.
    - `docs/DESIGN.md` §2.2 as amended by #118 — a chat's entrance into a world appears **when the chat activates in that world** (its first raid), not when the bot is added; this issue creates the chat, not its gates.
    - `docs/DESIGN.md` §16.7 — **the open question this issue closes.** Membership carries the backpack head start for one's own chat, contributions to buildings, "chat knowledge is the union across members" (§2.4), and the leaderboards. The technical constraint is hard: **the Bot API cannot enumerate a group's members** — membership is learned only by observation (a message in the chat, a Start from a deep link in a chat notification). The proposed default: membership accrues from events; a player may belong to several chats; **a raid always starts from a specific chat** and the session is bound to it, so the backpack head start and leaderboard credit go to the session's chat. For the solo MVP, "the player arrived via a deep link from chat X" is enough.
    - `docs/DESIGN.md` §2.1 — locations of dead chats become ruins. Not MVP; do not schema out the possibility.
    - `AGENTS.md` § Domain Rules — never write to a chat that is not the intended one.

    ## Depends on

    #21 #22 #28 #29

    ## Scope

    - Handling `my_chat_member` for bot added / removed, and creating the chat owner and its location. No gate is placed here — a gate appears at activation (#36, #29).
    - The deep-link Start flow: link generation carrying the chat reference, Start handling, player owner creation, and the membership record.
    - Membership accrual by observation, with the multi-chat case representable from day one.
    - The peaceful-home rule expressed where it cannot be bypassed.
    - Bot removed from chat: record it, stop notifying, and leave the data intact.
    - New enum members by migration where needed (`owner_kind` already has `chat` and `player`).

    ## Out of scope

    - Ruins of dead chats (§2.1) — not designed beyond the sentence, IDEAS.md territory.
    - Buildings and contributions (§6.2) — not MVP.
    - Leaderboards and achievements (§10) — not MVP.

    ## Telemetry obligation

    - Events: `bot_added_to_chat`, `player_started`, `bot_kicked` (§13.4). **`bot_kicked` is the terminal product metric — every occurrence gets a post-mortem** (§13.3), so its payload has to carry enough to hold one.
    - The activation funnel (§13.3, the MVP's headline number) is computed from these events plus `raid_started`.

    ## Open questions to close in the spec

    - What exactly makes a player a member: any observed message, or only a Start from that chat's deep link.
    - Which chat a player's raid starts from when they belong to several, and how the UI asks.

    ---

    Part of #47 (MVP roadmap).


  comments: []
  linked_issues: ["#21", "#22", "#28", "#29", "#36", "#47", "#118"]
  issue_body_status: current
  linked_prs: []
round_cap: 4
questions_per_round_cap: 3
round: 1
agent_id: add6904bcd7e3c350
prior_qa: []
```
