# Chat location, deep-link onboarding, and player-to-chat membership

**Source:** issue #30
**Date:** 2026-09-19
**Tracked in:** #30

The game's front door. A group chat that adds the bot becomes a settlement: it gets a
home of its own, peaceful by design. A player reaches the game from that chat — the
bot cannot write to anyone who has not pressed Start, so a chat notification carries a
link that opens the private chat and says which chat it came from. Taking that link is
the one act that makes a player a member of a chat; several later mechanics read what
it records, and the first of them lands here — what a chat knows of the world is the
union of what its members have discovered.

Two terms the rows below lean on. A chat's **home** is its location outside the mazes,
the collective entity a chat is in the game. **Membership** is the recorded link
between a player and a chat; the Bot API offers no way to enumerate a group's members,
so every such link comes from something the bot saw, and the owner has settled which
observation counts.

## Scope

1. A group chat that adds the bot becomes a chat in the game, with a home of its own. [task: "adding the bot to a chat creates that chat's location"]
2. A chat notification can hand a player a way into the game that says which chat it came from, and a player who takes it is in the game and linked to that chat. [task: "The deep-link Start flow: link generation carrying the chat reference, Start handling, player owner creation, and the membership record."]
3. A player becomes a member of a chat by pressing Start through that chat's link, and by nothing else the bot observes. [answer 1.1: "Только Start-ссылка"]
4. A player who is a member of more than one chat is representable from day one. [task: "with the multi-chat case representable from day one"]
5. What a chat knows of a maze is the union of what its members have discovered, and it follows membership as membership grows. [answer 1.3: "Отдаём здесь"]
6. A chat's home is a peaceful place, and it stays peaceful for a mechanic that never asked whether it was. [task: "The peaceful-home rule expressed where it cannot be bypassed."]
7. The bot's removal from a chat is recorded, ends the bot's traffic to that chat, and destroys nothing the chat accumulated. [task: "Bot removed from chat: record it, stop notifying, and leave the data intact."]
8. The mechanic's three events are recorded as they happen, and the removal event carries a post-mortem's worth of circumstance. [task: "Events: `bot_added_to_chat`, `player_started`, `bot_kicked`"]

## Out of scope

- A chat's gate — its entrance into a world. It appears when the chat activates there; #36 activates and #29 allocates. [task: "No gate is placed here — a gate appears at activation (#36, #29)."]
- The raid itself, and the binding of a raid session to the chat it started from — #36 applies the raid-start rule this spec settles.
- A membership ending. This task accrues memberships and ends none; §16.7 has membership accrue and names no way for it to lapse.
- The radar and the Mini App that render what a chat knows — not MVP. This task delivers the knowledge itself, not a surface that shows it.
- Ruins of the locations of chats that go quiet — not designed beyond a sentence.
- Buildings and contributions to them — not MVP.
- Leaderboards and achievements — not MVP.

## Deferred

_None._

## Key decisions

| Question | Decision |
|---|---|
| What makes a player a member of a chat | A Start pressed through that chat's link, and nothing else the bot sees. A message observed in the chat does not make its author a member — so every member is someone the bot can write to, and the solo-MVP minimum of DESIGN §16.7, the player arrived through chat X's link, is exactly what is recorded. [answer 1.1: "Только Start-ссылка"] |
| Which chat a raid starts from when the player is a member of several, and how the player is asked | The chat whose link or notification button led to the raid, and the player is never asked. Begun outside that path, a raid starts from the player's only chat, and is refused when the player has more than one. #36 applies the rule at raid start; what this task delivers is the chat each way in carries and the membership it records. [answer 1.2: "По ссылке"] |
| Whether what a chat knows — the union of its members' discoveries — lands with this task, as #29's approved spec deferred it here | Yes. It lands together with the membership it unions over, so chat knowledge accrues from the first discovery onward and no later task has to back-fill it. [answer 1.3: "Отдаём здесь"] |

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC1 | After the bot is added to a group chat, that chat exists in the game with a home of its own, and adding the bot to a second chat yields a second chat with a home of its own. [task: "adding the bot to a chat creates that chat's location"] |
| AC2 | A chat's home is a fully peaceful zone: no PvP and no looting can occur there, and the rule holds against a mechanic that fights or loots without asking whether the place is a home. [task: "the chat's home is a fully peaceful zone: no PvP, no looting"] |
| AC3 | Being added to a chat does not give that chat an entrance into any world. [task: "No gate is placed here — a gate appears at activation (#36, #29)."] |
| AC4 | A notification the bot sends to a chat can carry a link that opens the bot's private chat and identifies the chat it was sent from, for a player the bot has never been able to write to. [task: "a player reaches the game through a deep link from a chat notification"] |
| AC5 | A player who presses Start through such a link is a player in the game and is a member of the chat the link named. [task: "Start handling, player owner creation, and the membership record"] |
| AC6 | A player already in the game who presses Start through a second chat's link is a member of both chats, and neither membership replaces or weakens the other. [task: "a player may belong to several chats"] |
| AC7 | A second Start through a link for a chat the player is already a member of leaves one player and one membership of that chat. [task: "Start handling, player owner creation, and the membership record"] |
| AC8 | A player whose messages the bot sees in a chat, and who has not pressed Start through that chat's link, is not a member of that chat. [answer 1.1: "Только Start-ссылка"] |
| AC9 | A player who starts the bot without arriving through a chat's link is a player in the game and a member of no chat. [answer 1.1: "Только Start-ссылка"] |
| AC10 | What a chat knows of a maze is every cell discovered by a member of that chat, and nothing else. [answer 1.3: "Отдаём здесь"] |
| AC11 | A cell discovered by a player who is not a member of a given chat is not part of what that chat knows, however many other chats the player is a member of. [answer 1.3: "Отдаём здесь"] |
| AC12 | A player who becomes a member of a chat brings what they had already discovered into what that chat knows, and a player who is a member of two chats has each discovery counted in both. [answer 1.3: "Отдаём здесь"] |
| AC13 | After the bot is removed from a chat, the bot sends that chat nothing, and the chat, its home, every membership recorded for it and what it knows are still there. [task: "Bot removed from chat: record it, stop notifying, and leave the data intact."] |
| AC14 | A bot added again to a chat it had been removed from resumes against the chat that was already there, with what that chat accumulated intact and its traffic allowed again. [task: "Bot removed from chat: record it, stop notifying, and leave the data intact."] |
| AC15 | Adding the bot to a chat, a player's arrival in the game, and the bot's removal from a chat are each recorded in the event log as they happen. [task: "Events: `bot_added_to_chat`, `player_started`, `bot_kicked`"] |
| AC16 | The recorded removal answers on its own which chat the bot left, when, and by what act as far as the update reports it, so a post-mortem of the removal depends on nothing the removal ended. [task: "so its payload has to carry enough to hold one"] |
| AC17 | For a chat the bot was added to and a player who arrived through that chat's link, the activation funnel reports that chat with both steps attributed to it, from the recorded events alone. [task: "The activation funnel (§13.3, the MVP's headline number) is computed from these events plus `raid_started`."] |

## Open questions

_None._
