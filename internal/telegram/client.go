package telegram

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
)

type Client struct {
	client    *telegram.Client
	api       *tg.Client
	channelID int64
	mu        sync.Mutex
	ready     bool
}

func NewClient(apiID int, apiHash string, channelID int64) *Client {
	sessionStorage := &session.FileStorage{Path: "telegram.session"}

	client := telegram.NewClient(apiID, apiHash, telegram.Options{
		SessionStorage: sessionStorage,
	})

	return &Client{
		client:    client,
		channelID: channelID,
	}
}

func (c *Client) Start(ctx context.Context, phone string) error {
	return c.client.Run(ctx, func(ctx context.Context) error {
		status, err := c.client.Auth().Status(ctx)
		if err != nil {
			return fmt.Errorf("auth status: %w", err)
		}

		if !status.Authorized {
			flow := auth.NewFlow(
				terminalAuth{phone: phone},
				auth.SendCodeOptions{},
			)
			if err := c.client.Auth().IfNecessary(ctx, flow); err != nil {
				return fmt.Errorf("auth flow: %w", err)
			}
		}

		c.api = c.client.API()
		c.mu.Lock()
		c.ready = true
		c.mu.Unlock()

		log.Println("Telegram client connected and authenticated")

		<-ctx.Done()
		return ctx.Err()
	})
}

func (c *Client) IsReady() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ready
}

func (c *Client) UploadFile(ctx context.Context, filePath string) (int, error) {
	if !c.IsReady() {
		return 0, fmt.Errorf("telegram client not ready")
	}

	f, err := os.Open(filePath)
	if err != nil {
		return 0, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return 0, fmt.Errorf("stat file: %w", err)
	}

	u := uploader.NewUploader(c.api)
	uploaded, err := u.FromReader(ctx, filepath.Base(filePath), f)
	if err != nil {
		return 0, fmt.Errorf("upload to telegram: %w", err)
	}

	channel, err := c.resolveChannel(ctx)
	if err != nil {
		return 0, err
	}

	var media tg.InputMediaClass
	if stat.Size() > 10*1024*1024 {
		media = &tg.InputMediaUploadedDocument{
			File:     uploaded,
			MimeType: "application/octet-stream",
			Attributes: []tg.DocumentAttributeClass{
				&tg.DocumentAttributeFilename{FileName: filepath.Base(filePath)},
			},
		}
	} else {
		media = &tg.InputMediaUploadedDocument{
			File:     uploaded,
			MimeType: "application/octet-stream",
			Attributes: []tg.DocumentAttributeClass{
				&tg.DocumentAttributeFilename{FileName: filepath.Base(filePath)},
			},
		}
	}

	result, err := c.api.MessagesSendMedia(ctx, &tg.MessagesSendMediaRequest{
		Peer:   channel,
		Media:  media,
		RandomID: generateRandomID(),
	})
	if err != nil {
		return 0, fmt.Errorf("send media: %w", err)
	}

	return extractMessageID(result), nil
}

func (c *Client) DownloadFile(ctx context.Context, msgID int, w io.Writer) error {
	if !c.IsReady() {
		return fmt.Errorf("telegram client not ready")
	}

	channel, err := c.resolveChannel(ctx)
	if err != nil {
		return err
	}

	inputChannel, ok := channel.(*tg.InputPeerChannel)
	if !ok {
		return fmt.Errorf("not a channel")
	}

	msgs, err := c.api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
		Channel: &tg.InputChannel{
			ChannelID:  inputChannel.ChannelID,
			AccessHash: inputChannel.AccessHash,
		},
		ID: []tg.InputMessageClass{&tg.InputMessageID{ID: msgID}},
	})
	if err != nil {
		return fmt.Errorf("get message: %w", err)
	}

	messages, ok := msgs.(*tg.MessagesChannelMessages)
	if !ok || len(messages.Messages) == 0 {
		return fmt.Errorf("message not found")
	}

	msg, ok := messages.Messages[0].(*tg.Message)
	if !ok {
		return fmt.Errorf("unexpected message type")
	}

	d := downloader.NewDownloader()

	switch media := msg.Media.(type) {
	case *tg.MessageMediaDocument:
		document, ok := media.Document.(*tg.Document)
		if !ok {
			return fmt.Errorf("unexpected document type")
		}
		_, err = d.Download(c.api, &tg.InputDocumentFileLocation{
			ID:            document.ID,
			AccessHash:    document.AccessHash,
			FileReference: document.FileReference,
		}).Stream(ctx, w)
		return err

	case *tg.MessageMediaPhoto:
		photo, ok := media.Photo.(*tg.Photo)
		if !ok {
			return fmt.Errorf("unexpected photo type")
		}
		_, err = d.Download(c.api, &tg.InputPhotoFileLocation{
			ID:            photo.ID,
			AccessHash:    photo.AccessHash,
			FileReference: photo.FileReference,
			ThumbSize:     "y",
		}).Stream(ctx, w)
		return err

	default:
		return fmt.Errorf("unsupported media type")
	}
}

func (c *Client) DeleteMessage(ctx context.Context, msgID int) error {
	if !c.IsReady() {
		return fmt.Errorf("telegram client not ready")
	}

	channel, err := c.resolveChannel(ctx)
	if err != nil {
		return err
	}

	inputChannel, ok := channel.(*tg.InputPeerChannel)
	if !ok {
		return fmt.Errorf("not a channel")
	}

	_, err = c.api.ChannelsDeleteMessages(ctx, &tg.ChannelsDeleteMessagesRequest{
		Channel: &tg.InputChannel{
			ChannelID:  inputChannel.ChannelID,
			AccessHash: inputChannel.AccessHash,
		},
		ID: []int{msgID},
	})
	return err
}

func (c *Client) resolveChannel(ctx context.Context) (tg.InputPeerClass, error) {
	dialogs, err := c.api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
		OffsetPeer: &tg.InputPeerEmpty{},
		Limit:      100,
	})
	if err != nil {
		return nil, fmt.Errorf("get dialogs: %w", err)
	}

	var chats []tg.ChatClass
	switch d := dialogs.(type) {
	case *tg.MessagesDialogs:
		chats = d.Chats
	case *tg.MessagesDialogsSlice:
		chats = d.Chats
	default:
		return nil, fmt.Errorf("unexpected dialogs type")
	}

	targetID := c.channelID
	if targetID < 0 {
		s := fmt.Sprintf("%d", -targetID)
		if len(s) > 3 && s[:3] == "100" {
			stripped, _ := strconv.ParseInt(s[3:], 10, 64)
			targetID = stripped
		} else {
			targetID = -targetID
		}
	}

	for _, chat := range chats {
		ch, ok := chat.(*tg.Channel)
		if !ok {
			continue
		}
		if ch.ID == targetID {
			return &tg.InputPeerChannel{
				ChannelID:  ch.ID,
				AccessHash: ch.AccessHash,
			}, nil
		}
	}

	return nil, fmt.Errorf("channel %d not found (resolved target: %d, found %d chats)", c.channelID, targetID, len(chats))
}
