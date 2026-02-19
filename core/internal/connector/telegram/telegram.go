package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/h1v3-io/h1v3/internal/connector"
)

// Config holds Telegram connector configuration.
type Config struct {
	Token     string       // Bot token from @BotFather
	AllowFrom []int64      // Allowed Telegram user IDs (empty = allow all)
	Voice     *VoiceConfig // Optional voice transcription settings
}

// Connector implements the connector.Connector interface for Telegram.
type Connector struct {
	bot     *tgbotapi.BotAPI
	config  Config
	handler connector.InboundHandler
	logger  *slog.Logger
	cancel  context.CancelFunc
}

// New creates a new Telegram connector.
func New(cfg Config, handler connector.InboundHandler, logger *slog.Logger) (*Connector, error) {
	bot, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("telegram: init bot: %w", err)
	}

	if logger == nil {
		logger = slog.Default()
	}

	logger.Info("telegram bot authorized", "username", bot.Self.UserName)

	return &Connector{
		bot:     bot,
		config:  cfg,
		handler: handler,
		logger:  logger,
	}, nil
}

func (c *Connector) Name() string { return "telegram" }

// Start begins long-polling for updates. Blocks until context is cancelled.
func (c *Connector) Start(ctx context.Context) error {
	ctx, c.cancel = context.WithCancel(ctx)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30

	updates := c.bot.GetUpdatesChan(u)

	c.logger.Info("telegram connector started", "bot", c.bot.Self.UserName)

	for {
		select {
		case update := <-updates:
			if update.Message == nil {
				continue
			}
			c.handleUpdate(ctx, update)

		case <-ctx.Done():
			c.bot.StopReceivingUpdates()
			c.logger.Info("telegram connector stopped")
			return ctx.Err()
		}
	}
}

// Stop gracefully shuts down the connector.
func (c *Connector) Stop() error {
	if c.cancel != nil {
		c.cancel()
	}
	return nil
}

// Send delivers a message to a Telegram chat.
func (c *Connector) Send(_ context.Context, msg connector.OutboundMessage) error {
	chatID, err := strconv.ParseInt(msg.ChatID, 10, 64)
	if err != nil {
		return fmt.Errorf("telegram: invalid chat_id %q: %w", msg.ChatID, err)
	}

	if strings.TrimSpace(msg.Content) == "" {
		c.logger.Warn("skipping empty message", "chat_id", msg.ChatID)
		return nil
	}

	// Convert Markdown to Telegram HTML
	html := MarkdownToTelegramHTML(msg.Content)

	tgMsg := tgbotapi.NewMessage(chatID, html)
	tgMsg.ParseMode = "HTML"
	tgMsg.DisableWebPagePreview = true

	_, err = c.bot.Send(tgMsg)
	if err != nil {
		// Fallback to plain text if HTML fails
		c.logger.Warn("HTML send failed, falling back to plain text",
			"chat_id", msg.ChatID,
			"error", err,
		)
		tgMsg.Text = StripMarkdown(msg.Content)
		tgMsg.ParseMode = ""
		_, err = c.bot.Send(tgMsg)
	}

	return err
}

func (c *Connector) handleUpdate(ctx context.Context, update tgbotapi.Update) {
	msg := update.Message
	userID := msg.From.ID
	chatID := msg.Chat.ID

	// Access control
	if len(c.config.AllowFrom) > 0 && !contains(c.config.AllowFrom, userID) {
		c.logger.Warn("unauthorized user", "user_id", userID, "username", msg.From.UserName)
		return
	}

	// Handle commands
	if msg.IsCommand() {
		c.handleCommand(ctx, msg)
		return
	}

	// Extract text content
	text := msg.Text
	if text == "" && msg.Caption != "" {
		text = msg.Caption
	}

	// Handle voice/audio messages
	if text == "" && (msg.Voice != nil || msg.Audio != nil) {
		if c.config.Voice != nil && c.config.Voice.WhisperAPIKey != "" {
			transcribed, err := c.transcribeVoice(ctx, msg)
			if err != nil {
				c.logger.Error("voice transcription failed",
					"chat_id", chatID,
					"error", err,
				)
				reply := tgbotapi.NewMessage(chatID, "Sorry, I couldn't transcribe that voice message.")
				c.bot.Send(reply)
				return
			}
			text = "[Voice message]: " + transcribed
		}
	}

	if text == "" {
		return
	}

	// Send typing indicator
	typing := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	c.bot.Send(typing)

	// Forward to inbound handler
	inbound := connector.InboundMessage{
		Channel:  "telegram",
		SenderID: strconv.FormatInt(userID, 10),
		ChatID:   strconv.FormatInt(chatID, 10),
		Content:  text,
	}

	if err := c.handler(ctx, inbound); err != nil {
		c.logger.Error("inbound handler error",
			"chat_id", chatID,
			"error", err,
		)
	}
}

func (c *Connector) handleCommand(ctx context.Context, msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	switch msg.Command() {
	case "help":
		help := strings.Join([]string{
			"Available commands:",
			"/start — Start the bot",
			"/new — Start a new conversation",
			"/help — Show this help message",
			"",
			"Just send me a message to chat!",
		}, "\n")
		reply := tgbotapi.NewMessage(chatID, help)
		c.bot.Send(reply)

	default:
		// Forward all other commands (including /start, /new) through the handler
		// so the FrontAgent can manage session lifecycle.
		text := "/" + msg.Command()
		if msg.CommandArguments() != "" {
			text += " " + msg.CommandArguments()
		}
		inbound := connector.InboundMessage{
			Channel:  "telegram",
			SenderID: strconv.FormatInt(msg.From.ID, 10),
			ChatID:   strconv.FormatInt(chatID, 10),
			Content:  text,
		}
		c.handler(ctx, inbound)
	}
}

func contains(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}
