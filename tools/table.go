// Per-protocol facts. Every value is cited in docs/protocol-table.md.
package main

const (
	era759   = iota + 1 // 1.19: cmd, ts, salt, sig array, signedPreview
	era760              // 1.19.1/1.19.2: era759 + previousMessages + lastRejected
	era761              // 1.19.3-1.20.4: cmd, ts, salt, sig array, msgCount, 3-byte ack bitset
	eraPlain            // 1.20.5+: String command is the whole packet
	eraChat             // <=1.18.2: plain chat packet, slash INCLUDED in the string
)

type row struct {
	proto      int
	name       string // human label for diagnostics
	era        int
	kaCB, kaSB int // play-state keep_alive S->C / C->S
	cmdID      int // serverbound chat_command (chat on <=1.18.2) id
}

var rows = map[int]row{
	// pre-1.19: no configuration state, no login_acknowledged, command goes
	// out as a chat packet with the slash in the string.
	477: {477, "1.14", eraChat, 0x20, 0x0F, 0x03},
	498: {498, "1.14.4", eraChat, 0x20, 0x0F, 0x03},
	578: {578, "1.15.2", eraChat, 0x21, 0x0F, 0x03},
	735: {735, "1.16", eraChat, 0x20, 0x10, 0x03},
	736: {736, "1.16.1", eraChat, 0x20, 0x10, 0x03},
	751: {751, "1.16.2", eraChat, 0x1F, 0x10, 0x03},
	753: {753, "1.16.3", eraChat, 0x1F, 0x10, 0x03},
	754: {754, "1.16.4/1.16.5", eraChat, 0x1F, 0x10, 0x03},
	755: {755, "1.17", eraChat, 0x21, 0x0F, 0x03},
	756: {756, "1.17.1", eraChat, 0x21, 0x0F, 0x03},
	757: {757, "1.18/1.18.1", eraChat, 0x21, 0x0F, 0x03},
	758: {758, "1.18.2", eraChat, 0x21, 0x0F, 0x03},
	759: {759, "1.19", era759, 0x1E, 0x11, 0x03},
	760: {760, "1.19.1/1.19.2", era760, 0x20, 0x12, 0x04},
	761: {761, "1.19.3", era761, 0x1F, 0x11, 0x04},
	762: {762, "1.19.4", era761, 0x23, 0x12, 0x04},
	763: {763, "1.20/1.20.1", era761, 0x23, 0x12, 0x04},
	764: {764, "1.20.2", era761, 0x24, 0x14, 0x04},
	765: {765, "1.20.3/1.20.4", era761, 0x24, 0x15, 0x04},
	766: {766, "1.20.5/1.20.6", eraPlain, 0x26, 0x18, 0x04},
	767: {767, "1.21/1.21.1", eraPlain, 0x26, 0x18, 0x04},
	768: {768, "1.21.2/1.21.3", eraPlain, 0x27, 0x1A, 0x05},
	769: {769, "1.21.4", eraPlain, 0x27, 0x1A, 0x05},
	770: {770, "1.21.5", eraPlain, 0x26, 0x1A, 0x05},
	771: {771, "1.21.6", eraPlain, 0x26, 0x1B, 0x06},
	772: {772, "1.21.7/1.21.8", eraPlain, 0x26, 0x1B, 0x06},
	773: {773, "1.21.9/1.21.10", eraPlain, 0x2B, 0x1B, 0x06},
	774: {774, "1.21.11", eraPlain, 0x2B, 0x1B, 0x06},
	775: {775, "26.1", eraPlain, 0x2C, 0x1C, 0x07},
	776: {776, "26.2", eraPlain, 0x2C, 0x1C, 0x07},
}

func rowFor(proto int) (row, bool) {
	r, ok := rows[proto]
	return r, ok
}

// config reports whether the version has the 1.20.2+ configuration state.
func (r row) config() bool { return r.proto >= 764 }

// cfgIDs are the configuration-state packet ids; -1 means the packet does not
// exist on that version (and never matches a received id).
type cfgIDs struct {
	discCB, finishCB, kaCB, pingCB, knownCB, cocCB int // clientbound
	kaSB, pongSB, finishSB, knownSB, acceptSB      int // serverbound
}

func (r row) cfg() cfgIDs {
	if r.proto >= 766 {
		// cookie_request was inserted at the front in 1.20.5, shifting
		// everything by one; select_known_packs is 1.20.5+, code_of_conduct
		// appears in dumps at 1.21.9 and is handled defensively everywhere.
		return cfgIDs{
			discCB: 0x02, finishCB: 0x03, kaCB: 0x04, pingCB: 0x05, knownCB: 0x0E, cocCB: 0x13,
			kaSB: 0x04, pongSB: 0x05, finishSB: 0x03, knownSB: 0x07, acceptSB: 0x09,
		}
	}
	// 1.20.2-1.20.4 (764-765)
	return cfgIDs{
		discCB: 0x01, finishCB: 0x02, kaCB: 0x03, pingCB: 0x04, knownCB: -1, cocCB: -1,
		kaSB: 0x03, pongSB: 0x04, finishSB: 0x02, knownSB: -1, acceptSB: -1,
	}
}
