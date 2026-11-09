package telegram

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gotd/td/tg"
)

type SyncedFile struct {
	MsgID    int
	FileName string
	FileSize int64
	MimeType string
	Date     time.Time
}

func (c *Client) GetChannelMessages(ctx context.Context, offsetMsgID int, limit int) ([]SyncedFile, error) {
	if !c.IsReady() {
		return nil, fmt.Errorf("telegram client not ready")
	}

	channel, err := c.resolveChannel(ctx)
	if err != nil {
		return nil, err
	}

	inputChannel, ok := channel.(*tg.InputPeerChannel)
	if !ok {
		return nil, fmt.Errorf("not a channel")
	}

	history, err := c.api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:      channel,
		Limit:     limit,
		OffsetID:  offsetMsgID,
		AddOffset: 0,
	})
	if err != nil {
		return nil, fmt.Errorf("get history: %w", err)
	}

	_ = inputChannel

	var messages []tg.MessageClass
	switch h := history.(type) {
	case *tg.MessagesMessages:
		messages = h.Messages
	case *tg.MessagesMessagesSlice:
		messages = h.Messages
	case *tg.MessagesChannelMessages:
		messages = h.Messages
	default:
		return nil, fmt.Errorf("unexpected history type")
	}

	var files []SyncedFile
	log.Printf("Sync: fetched %d messages from channel", len(messages))

	for _, m := range messages {
		msg, ok := m.(*tg.Message)
		if !ok {
			continue
		}

		if msg.Media == nil {
			continue
		}

		var sf *SyncedFile

		switch media := msg.Media.(type) {
		case *tg.MessageMediaDocument:
			document, ok := media.Document.(*tg.Document)
			if !ok {
				continue
			}
			fileName := "unknown"
			mimeType := document.MimeType
			for _, attr := range document.Attributes {
				if fn, ok := attr.(*tg.DocumentAttributeFilename); ok {
					fileName = fn.FileName
				}
			}
			sf = &SyncedFile{
				MsgID:    msg.ID,
				FileName: fileName,
				FileSize: document.Size,
				MimeType: mimeType,
				Date:     time.Unix(int64(msg.Date), 0),
			}

		case *tg.MessageMediaPhoto:
			photo, ok := media.Photo.(*tg.Photo)
			if !ok {
				continue
			}
			var maxSize int64
			for _, size := range photo.Sizes {
				if s, ok := size.(*tg.PhotoSize); ok {
					if int64(s.Size) > maxSize {
						maxSize = int64(s.Size)
					}
				}
			}
			sf = &SyncedFile{
				MsgID:    msg.ID,
				FileName: fmt.Sprintf("photo_%d.jpg", msg.ID),
				FileSize: maxSize,
				MimeType: "image/jpeg",
				Date:     time.Unix(int64(msg.Date), 0),
			}
		}

		if sf != nil {
			files = append(files, *sf)
		}
	}

	log.Printf("Sync: found %d files in channel", len(files))
	return files, nil
}

type SyncCallback func(file SyncedFile) error

func (c *Client) StartSync(ctx context.Context, interval time.Duration, lastKnownMsgID int, callback SyncCallback) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	currentLastID := lastKnownMsgID

	log.Printf("Telegram sync started (interval: %s, last known msg ID: %d)", interval, currentLastID)

	for {
		select {
		case <-ctx.Done():
			log.Println("Telegram sync stopped")
			return
		case <-ticker.C:
			files, err := c.GetChannelMessages(ctx, 0, 50)
			if err != nil {
				log.Printf("Sync error: %v", err)
				continue
			}

			for i := len(files) - 1; i >= 0; i-- {
				f := files[i]
				if f.MsgID <= currentLastID {
					continue
				}
				if err := callback(f); err != nil {
					log.Printf("Sync callback error for msg %d: %v", f.MsgID, err)
					continue
				}
				currentLastID = f.MsgID
				log.Printf("Synced file: %s (msg_id: %d)", f.FileName, f.MsgID)
			}
		}
	}
}
