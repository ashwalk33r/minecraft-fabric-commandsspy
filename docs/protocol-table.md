# Per-version protocol table, 1.14 – 26.2

Protocol reference for the Go e2e bot (`tools/`). Scope: every version the e2e grid can
boot on any event — the Makefile's default `VERSIONS` list, plus the versions the
workflow_dispatch deep sweep adds. Note that a `.0` release is its own protocol and needs
its own row: 1.14 is 477 and 1.14.4 is 498; 1.16 is 735 and 1.16.1 is 736; 1.17 is 755 and
1.17.1 is 756. Missing rows for those three cost the deep sweep six red legs on its second
run — the server booted and the mod loaded, but the bot could not join, so only the
player-command assertion failed.

Only the facts a login + one-command + disconnect client needs. All IDs are serverbound (C→S) or clientbound (S→C)
as labelled; all values hex packet ids unless stated. `tools/table.go` implements this
table.

Sources, abbreviated in the tables:
- **md `pc/X`** — [PrismarineJS/minecraft-data](https://github.com/PrismarineJS/minecraft-data)
  `data/pc/X/protocol.json`, resolved through `data/dataPaths.json`.
  Some versions alias to a neighbouring dump; the alias is named where it applies.
- **wiki (776)** — <https://minecraft.wiki/w/Java_Edition_protocol/Packets>, which documents
  protocol 776 / Minecraft 26.2.
- **probed** — established empirically against a live server. minecraft-data has **no 26.2
  dump**, so the 26.2 row rests on wiki + live probing; re-probe it on any change.

## Cross-range constants (stated once, not tabulated)

Constant for the entire 1.14.4 (498) → 26.2 (776) range, verified at both ends:

| Packet | State / direction | Id | Verified at 1.14.4 | Verified at 26.2 |
|---|---|---|---|---|
| Login Start | login C→S | `0x00` | md `pc/1.14.4` | md `pc/26.1` = wiki (776); exercised by live login |
| Login Success | login S→C | `0x02` | md `pc/1.14.4` | md `pc/26.1` = wiki (776); exercised live |
| Set Compression | login S→C | `0x03` | md `pc/1.14.4` | md `pc/26.1` = wiki (776) |
| Login Acknowledged | login C→S | `0x03` — **exists only from 1.20.2 (764)** | n/a (absent, md `pc/1.14.4` has no `login_acknowledged`) | md `pc/26.1` = wiki (776); exercised live |

Also constant: handshake C→S `0x00`, VarInt length-prefix framing, Keep Alive body `i64`
both directions, Login Disconnect S→C `0x00`.

## Main table

Columns: **cfg** = has configuration state (1.20.2+); **UUID** = Login Start UUID mode
(none / opt = optional trailing `Option<UUID>` / req = required raw UUID); **sig** = chat
signature mode (see era staircase below); **cmd** = the serverbound command packet
(`chat` ≤1.18.2, `chat_command` 1.19+) and its id; **KA play** = play-state Keep Alive
S→C / C→S; **login(play)** = clientbound play `login` (Join Game), the "we reached play"
trigger.

| MC | proto | cfg | UUID | sig | cmd pkt | cmd id | KA play S→C/C→S | login(play) | source |
|---|---|---|---|---|---|---|---|---|---|
| 1.14 | 477 | no | none | — | chat | 0x03 | 0x20 / 0x0F | 0x25 | md `pc/1.14` |
| 1.14.4 | 498 | no | none | — | chat | 0x03 | 0x20 / 0x0F | 0x25 | md `pc/1.14.4` |
| 1.15.2 | 578 | no | none | — | chat | 0x03 | 0x21 / 0x0F | 0x26 | md `pc/1.15.2` |
| 1.16 | 735 | no | none | — | chat | 0x03 | 0x20 / 0x10 | 0x25 | md `pc/1.16` |
| 1.16.1 | 736 | no | none | — | chat | 0x03 | 0x20 / 0x10 | 0x25 | md `pc/1.16.1` |
| 1.16.2 | 751 | no | none | — | chat | 0x03 | 0x1F / 0x10 | 0x24 | md `pc/1.16.2` |
| 1.16.3 | 753 | no | none | — | chat | 0x03 | 0x1F / 0x10 | 0x24 | md `pc/1.16.2` (alias; no packet-id change 751→753) |
| 1.16.5 | 754 | no | none | — | chat | 0x03 | 0x1F / 0x10 | 0x24 | md `pc/1.16.2` (alias) |
| 1.17 | 755 | no | none | — | chat | 0x03 | 0x21 / 0x0F | 0x26 | md `pc/1.17` |
| 1.17.1 | 756 | no | none | — | chat | 0x03 | 0x21 / 0x0F | 0x26 | md `pc/1.17.1` |
| 1.18, 1.18.1 | 757 | no | none | — | chat | 0x03 | 0x21 / 0x0F | 0x26 | md `pc/1.18` |
| 1.18.2 | 758 | no | none | — | chat | 0x03 | 0x21 / 0x0F | 0x26 | md `pc/1.18.2` |
| 1.19.2 | 760 | no | opt (+sig opt) | era-760 | chat_command | 0x04 | 0x20 / 0x12 | 0x25 | md `pc/1.19.2` |
| 1.19.4 | 762 | no | opt | era-761 | chat_command | 0x04 | 0x23 / 0x12 | 0x28 | md `pc/1.19.4` |
| 1.20.1 | 763 | no | opt | era-761 | chat_command | 0x04 | 0x23 / 0x12 | 0x28 | md `pc/1.20` (alias) |
| 1.20.2 | 764 | yes | req | era-761 | chat_command | 0x04 | 0x24 / 0x14 | 0x29 | md `pc/1.20.2` |
| 1.20.3 | 765 | yes | req | era-761 | chat_command | 0x04 | 0x24 / 0x15 | 0x29 | md `pc/1.20.3` |
| 1.20.4 | 765 | yes | req | era-761 | chat_command | 0x04 | 0x24 / 0x15 | 0x29 | md `pc/1.20.3` (alias) |
| 1.20.5 | 766 | yes | req | none | chat_command | 0x04 | 0x26 / 0x18 | 0x2B | md `pc/1.20.5` |
| 1.20.6 | 766 | yes | req | none | chat_command | 0x04 | 0x26 / 0x18 | 0x2B | md `pc/1.20.5` (alias) |
| 1.21 | 767 | yes | req | none | chat_command | 0x04 | 0x26 / 0x18 | 0x2B | md `pc/1.21.1` (alias) |
| 1.21.1 | 767 | yes | req | none | chat_command | 0x04 | 0x26 / 0x18 | 0x2B | md `pc/1.21.1` |
| 1.21.2 | 768 | yes | req | none | chat_command | 0x05 | 0x27 / 0x1A | 0x2C | md `pc/1.21.3` (no 1.21.2 dump; same protocol 768) |
| 1.21.3 | 768 | yes | req | none | chat_command | 0x05 | 0x27 / 0x1A | 0x2C | md `pc/1.21.3` |
| 1.21.4 | 769 | yes | req | none | chat_command | 0x05 | 0x27 / 0x1A | 0x2C | md `pc/1.21.4` |
| 1.21.5 | 770 | yes | req | none | chat_command | 0x05 | 0x26 / 0x1A | 0x2B | md `pc/1.21.5` |
| 1.21.6 | 771 | yes | req | none | chat_command | 0x06 | 0x26 / 0x1B | 0x2B | md `pc/1.21.6` |
| 1.21.7 | 772 | yes | req | none | chat_command | 0x06 | 0x26 / 0x1B | 0x2B | md `pc/1.21.8` (no 1.21.7 dump; same protocol 772) |
| 1.21.8 | 772 | yes | req | none | chat_command | 0x06 | 0x26 / 0x1B | 0x2B | md `pc/1.21.8` |
| 1.21.9 | 773 | yes | req | none | chat_command | 0x06 | 0x2B / 0x1B | 0x30 | md `pc/1.21.9` |
| 1.21.10 | 773 | yes | req | none | chat_command | 0x06 | 0x2B / 0x1B | 0x30 | md `pc/1.21.9` (alias) |
| 1.21.11 | 774 | yes | req | none | chat_command | 0x06 | 0x2B / 0x1B | 0x30 | md `pc/1.21.11` |
| 26.1 | 775 | yes | req | none | chat_command | 0x07 | 0x2C / 0x1C | 0x31 | md `pc/26.1` |
| **26.2** | **776** | yes | req | none | chat_command | **0x07** | **0x2C / 0x1C** | **0x31** | wiki (776) + **probed** |

All protocol numbers verified against minecraft-data
`data/pc/common/protocolVersions.json`. Shared-protocol pairs (1.20.3/1.20.4,
1.20.5/1.20.6, 1.21/1.21.1, 1.21.2/1.21.3, 1.21.7/1.21.8, 1.21.9/1.21.10) have identical
packet layouts — one branch covers each pair.

## Configuration state (1.20.2+ only)

Configuration has its **own** Keep Alive ids — the keepalive loop must be state-aware.
Flow: Login Success → C→S Login Acknowledged `0x03` → configuration → S→C Finish
Configuration → C→S Acknowledge Finish Configuration → play.

| MC (proto) | finish S→C / C→S | KA cfg S→C / C→S | select_known_packs S→C / C→S | code_of_conduct S→C / accept C→S | source |
|---|---|---|---|---|---|
| 1.20.2 (764) | 0x02 / 0x02 | 0x03 / 0x03 | — | — | md `pc/1.20.2` |
| 1.20.3, 1.20.4 (765) | 0x02 / 0x02 | 0x03 / 0x03 | — | — | md `pc/1.20.3` |
| 1.20.5, 1.20.6 (766) | 0x03 / 0x03 | 0x04 / 0x04 | 0x0E / 0x07 | — | md `pc/1.20.5` |
| 1.21, 1.21.1 (767) | 0x03 / 0x03 | 0x04 / 0x04 | 0x0E / 0x07 | — | md `pc/1.21.1` |
| 1.21.2–1.21.4 (768–769) | 0x03 / 0x03 | 0x04 / 0x04 | 0x0E / 0x07 | — | md `pc/1.21.3`, `pc/1.21.4` |
| 1.21.5–1.21.8 (770–772) | 0x03 / 0x03 | 0x04 / 0x04 | 0x0E / 0x07 | — | md `pc/1.21.5`, `pc/1.21.6`, `pc/1.21.8` |
| 1.21.9–1.21.11 (773–774) | 0x03 / 0x03 | 0x04 / 0x04 | 0x0E / 0x07 | 0x13 / 0x09 | md `pc/1.21.9`, `pc/1.21.11` |
| 26.1 (775) | 0x03 / 0x03 | 0x04 / 0x04 | 0x0E / 0x07 | 0x13 / 0x09 | md `pc/26.1` |
| 26.2 (776) | 0x03 / 0x03 | 0x04 / 0x04 | 0x0E / 0x07 | 0x13 / 0x09 | wiki (776); finish/KA exercised by live login |

Everything shifted by one between 764/765 and 766+ because `cookie_request 0x00` was
inserted at the front of the clientbound list in 1.20.5 — hence finish `0x02` → `0x03`.
`select_known_packs` (1.20.5+): server sends its datapack list and waits; reply with an
empty array (`VarInt 0`) and it sends full registry data, which we ignore.

## Chat era staircase (5 eras)

The serverbound command packet's shape:

| Range | Packet | Fields |
|---|---|---|
| 1.14.4–1.18.2 | `chat` | `String message` — leading `/` **included** in the string |
| 1.19 (759) | `chat_command` | command, i64 timestamp, i64 salt, signature array, bool signedPreview — 1.19.0 itself is unsupported by the mod (see the wiki's [Version boundaries and root causes](https://github.com/ashwalk33r/minecraft-fabric-commandsspy/wiki/Version-Boundaries-And-Root-Causes)), so this row is live-untested |
| 1.19.1/1.19.2 (760) | `chat_command` | the above + previousMessages array + `Option<lastRejectedMessage>` — the "era-760" mode above |
| 1.19.3–1.20.4 (761–765) | `chat_command` | command, timestamp, salt, sig array, VarInt messageCount, 3-byte acknowledged bitset — "era-761" |
| **1.20.5+ (766+)** | `chat_command` | **`String command` — the whole packet.** Signing moved to a separate `chat_command_signed` |

Signed-era servers accept the unsigned form in offline mode (no profile keys, so
`enforce-secure-profile` is inert); verified live on 1.19.2. Wire layout used: empty sig
array, `signedPreview=false`, zero previous messages, no last rejected message.

**Do not confuse `chat_command` with `chat_command_signed`** (1.20.5+): signed is 0x05 (766),
0x06 (768), 0x07 (771), 0x08 (775/776) — always `chat_command + 1` in the dumps so far, and
sending it instead of the unsigned one gets the connection closed.

## The 26.2 row: probed, not copied

minecraft-data has no 26.2 dump (`dataPaths.json` has no `26.2` key). The row above comes
from minecraft.wiki's protocol-776 packet page **and** was confirmed against a live 26.2
Fabric server (protocol 776, Temurin 25) by id-probing:

| chat_command id tried | result |
|---|---|
| 0x06 | server closed the connection (EOF) |
| **0x07** | **`SERVER REPLY: commands.list.players`** — command executed |
| 0x08 | server closed the connection (EOF) — that id is `chat_command_signed` |

So 26.2 `chat_command` = `0x07`, body `String command`, identical to 26.1; keepalive
0x1C (C→S) / 0x2C (S→C) and play login 0x31 were exercised by the same successful live run.
Any future doubt about this row is settled by re-running the probe, not by re-reading this
table.

## code_of_conduct introduction

The minecraft-data dumps narrow the introduction: `code_of_conduct` / `accept_code_of_conduct`
are **absent in `pc/1.21.8` (protocol 772) and present in `pc/1.21.9` (protocol 773)**, so
the dump-level introduction is 1.21.9. This is dump evidence, not a runtime bisect; the
client handles it defensively regardless — on any config-state version, if the server sends
`code_of_conduct`, ack with `accept_code_of_conduct`. Vanilla defaults to no CoC configured,
so the packet normally never arrives.
