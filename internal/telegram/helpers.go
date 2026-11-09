package telegram

import (
	"crypto/rand"
	"encoding/binary"

	"github.com/gotd/td/tg"
)

func generateRandomID() int64 {
	var id int64
	binary.Read(rand.Reader, binary.LittleEndian, &id)
	if id < 0 {
		id = -id
	}
	return id
}

func extractMessageID(updates tg.UpdatesClass) int {
	switch u := updates.(type) {
	case *tg.Updates:
		for _, update := range u.Updates {
			if msg, ok := update.(*tg.UpdateNewChannelMessage); ok {
				if m, ok := msg.Message.(*tg.Message); ok {
					return m.ID
				}
			}
			if msg, ok := update.(*tg.UpdateNewMessage); ok {
				if m, ok := msg.Message.(*tg.Message); ok {
					return m.ID
				}
			}
		}
	}
	return 0
}
