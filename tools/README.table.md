# table.go

The protocol fact sheet for the test bot. One file, zero logic beyond lookups. Every number here is a packet id or era flag for a specific Minecraft protocol version, and every value is cited in `docs/protocol-table.md`.

## Why it exists

Minecraft changes packet ids almost every release. The bot (`bot.go`, `mc.go`) needs to speak many protocol versions, so all version-specific facts live in this one table instead of being scattered through the connection code.

## Eras

Constants `era759`, `era760`, `era761`, `eraPlain`, `eraChat` classify how a command is sent over the wire:

- `eraChat` — 1.18.2 and older. Commands go out as a plain chat packet, slash included in the string.
- `era759` — 1.19. Command packet with timestamp, salt, signature array, signedPreview.
- `era760` — 1.19.1/1.19.2. Same plus previousMessages and lastRejected.
- `era761` — 1.19.3 through 1.20.4. Signature fields plus msgCount and a 3-byte ack bitset.
- `eraPlain` — 1.20.5+. The command string is the whole packet.

`mc.go` switches on the era to build the right command packet.

## Types and functions

- `row` — one version's facts: protocol number, human-readable name, era, play-state keep_alive ids (both directions), and the serverbound command packet id.
- `rows` — map from protocol number to `row`. Covers 498 (1.14.4) through 776 (26.2).
- `rowFor(proto)` — lookup with an ok flag. `bot.go` calls it once at connect time; unknown protocol means unsupported version.
- `row.config()` — true for 764+ (1.20.2 introduced the configuration state).
- `cfgIDs` — configuration-state packet ids, clientbound and serverbound. `-1` means the packet does not exist on that version, chosen so it can never match a real received id.
- `row.cfg()` — returns the right `cfgIDs` set: one layout for 764-765, a shifted one for 766+ (cookie_request was inserted at the front in 1.20.5).

## Fit in the package

Pure data, no I/O, no state. `bot.go` resolves the `row` for the target server; `mc.go` reads era, keep_alive ids, and `cfg()` ids from it while driving the connection. To support a new Minecraft version, add one line to `rows` (and adjust `cfg()` only if Mojang moves configuration-state ids again).
