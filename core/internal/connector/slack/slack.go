package slackconn

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/h1v3-io/h1v3/internal/connector"
)

// Config holds Slack connector configuration.
type Config struct {
	BotToken string   // xoxb-... Bot User OAuth Token
	AppToken string   // xapp-... App-Level Token (for Socket Mode)
	Channels []string // Optional: only respond in these channels (empty = all)
}

// Connector implements connector.Connector for Slack via Socket Mode.
type Connector struct {
	api     *slack.Client
	socket  *socketmode.Client
	config  Config
	handler connector.InboundHandler
	logger  *slog.Logger
	cancel  context.CancelFunc
	botID   string
}

// New creates a new Slack connector.
func New(cfg Config, handler connector.InboundHandler, logger *slog.Logger) (*Connector, error) {
	if cfg.BotToken == "" {
		return nil, fmt.Errorf("slack: bot_token is required")
	}
	if cfg.AppToken == "" {
		return nil, fmt.Errorf("slack: app_token is required (Socket Mode)")
	}

	if logger == nil {
		logger = slog.Default()
	}

	api := slack.New(cfg.BotToken, slack.OptionAppLevelToken(cfg.AppToken))

	// Test auth and get bot user ID
	authResp, err := api.AuthTest()
	if err != nil {
		return nil, fmt.Errorf("slack: auth test: %w", err)
	}

	logger.Info("slack bot authorized", "user", authResp.User, "team", authResp.Team)

	socket := socketmode.New(api)

	return &Connector{
		api:     api,
		socket:  socket,
		config:  cfg,
		handler: handler,
		logger:  logger,
		botID:   authResp.UserID,
	}, nil
}

func (c *Connector) Name() string { return "slack" }

// Start begins listening for events via Socket Mode. Blocks until context is cancelled.
func (c *Connector) Start(ctx context.Context) error {
	ctx, c.cancel = context.WithCancel(ctx)

	go c.handleEvents(ctx)

	c.logger.Info("slack connector started (socket mode)")
	return c.socket.RunContext(ctx)
}

// Stop gracefully shuts down the connector.
func (c *Connector) Stop() error {
	if c.cancel != nil {
		c.cancel()
	}
	return nil
}

// Send delivers a message to a Slack channel.
func (c *Connector) Send(_ context.Context, msg connector.OutboundMessage) error {
	text := MarkdownToMrkdwn(msg.Content)

	opts := []slack.MsgOption{
		slack.MsgOptionText(text, false),
	}

	_, _, err := c.api.PostMessage(msg.ChatID, opts...)
	if err != nil {
		return fmt.Errorf("slack: send message: %w", err)
	}
	return nil
}

func (c *Connector) handleEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-c.socket.Events:
			switch event.Type {
			case socketmode.EventTypeEventsAPI:
				c.handleEventsAPI(ctx, event)
			case socketmode.EventTypeSlashCommand:
				c.handleSlashCommand(ctx, event)
			}
		}
	}
}

func (c *Connector) handleEventsAPI(ctx context.Context, event socketmode.Event) {
	eventsAPIEvent, ok := event.Data.(slackevents.EventsAPIEvent)
	if !ok {
		return
	}

	c.socket.Ack(*event.Request)

	switch ev := eventsAPIEvent.InnerEvent.Data.(type) {
	case *slackevents.MessageEvent:
		c.handleMessage(ctx, ev)
	case *slackevents.AppMentionEvent:
		c.handleMention(ctx, ev)
	}
}

func (c *Connector) handleMessage(ctx context.Context, ev *slackevents.MessageEvent) {
	// Ignore bot messages (including our own)
	if ev.BotID != "" || ev.User == "" || ev.User == c.botID {
		return
	}
	// Ignore message subtypes (edits, deletes, etc.)
	if ev.SubType != "" {
		return
	}

	// Channel filter
	if !c.isAllowedChannel(ev.Channel) {
		return
	}

	text := ev.Text
	if text == "" {
		return
	}

	// Use thread_ts as chat ID for thread grouping, fall back to channel
	chatID := ev.Channel
	if ev.ThreadTimeStamp != "" {
		chatID = ev.Channel + ":" + ev.ThreadTimeStamp
	}

	inbound := connector.InboundMessage{
		Channel:  "slack",
		SenderID: ev.User,
		ChatID:   chatID,
		Content:  text,
	}

	if err := c.handler(ctx, inbound); err != nil {
		c.logger.Error("slack inbound handler error",
			"channel", ev.Channel,
			"user", ev.User,
			"error", err,
		)
	}
}

func (c *Connector) handleMention(ctx context.Context, ev *slackevents.AppMentionEvent) {
	if ev.User == c.botID {
		return
	}

	// Strip the bot mention from the text
	text := StripMention(ev.Text, c.botID)
	if text == "" {
		return
	}

	chatID := ev.Channel
	if ev.ThreadTimeStamp != "" {
		chatID = ev.Channel + ":" + ev.ThreadTimeStamp
	}

	inbound := connector.InboundMessage{
		Channel:  "slack",
		SenderID: ev.User,
		ChatID:   chatID,
		Content:  text,
	}

	if err := c.handler(ctx, inbound); err != nil {
		c.logger.Error("slack mention handler error",
			"channel", ev.Channel,
			"user", ev.User,
			"error", err,
		)
	}
}

func (c *Connector) handleSlashCommand(ctx context.Context, event socketmode.Event) {
	cmd, ok := event.Data.(slack.SlashCommand)
	if !ok {
		return
	}

	c.socket.Ack(*event.Request)

	text := cmd.Text
	if text == "" {
		text = cmd.Command
	}

	inbound := connector.InboundMessage{
		Channel:  "slack",
		SenderID: cmd.UserID,
		ChatID:   cmd.ChannelID,
		Content:  text,
	}

	if err := c.handler(ctx, inbound); err != nil {
		c.logger.Error("slack slash command error",
			"command", cmd.Command,
			"user", cmd.UserID,
			"error", err,
		)
	}
}

func (c *Connector) isAllowedChannel(channel string) bool {
	if len(c.config.Channels) == 0 {
		return true
	}
	for _, ch := range c.config.Channels {
		if ch == channel {
			return true
		}
	}
	return false
}

// StripMention removes the <@BOTID> mention from message text.
func StripMention(text, botID string) string {
	mention := fmt.Sprintf("<@%s>", botID)
	text = strings.Replace(text, mention, "", 1)
	return strings.TrimSpace(text)
}

// MarkdownToMrkdwn converts standard Markdown to Slack's mrkdwn format.
func MarkdownToMrkdwn(md string) string {
	result := md

	// Convert emphasis markers in a single pass
	result = convertEmphasis(result)
	// Convert strikethrough: ~~text~~ → ~text~
	result = strings.ReplaceAll(result, "~~", "~")
	// Convert links: [text](url) → <url|text>
	result = convertLinks(result)

	return result
}

// convertEmphasis handles both bold (**text** → *text*) and italic (*text* → _text_)
// in a single pass, correctly distinguishing between the two.
func convertEmphasis(s string) string {
	var b strings.Builder
	inCode := false
	i := 0
	for i < len(s) {
		ch := s[i]
		if ch == '`' {
			inCode = !inCode
			b.WriteByte(ch)
			i++
		} else if ch == '*' && !inCode {
			if i+1 < len(s) && s[i+1] == '*' {
				// Bold: ** → * (Slack bold)
				b.WriteByte('*')
				i += 2
			} else {
				// Italic: * → _ (Slack italic)
				b.WriteByte('_')
				i++
			}
		} else {
			b.WriteByte(ch)
			i++
		}
	}
	return b.String()
}

// convertLinks converts [text](url) to <url|text>.
func convertLinks(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '[' {
			closeB := strings.Index(s[i:], "](")
			if closeB == -1 {
				b.WriteByte(s[i])
				i++
				continue
			}
			closeB += i
			closeP := strings.Index(s[closeB:], ")")
			if closeP == -1 {
				b.WriteByte(s[i])
				i++
				continue
			}
			closeP += closeB

			text := s[i+1 : closeB]
			url := s[closeB+2 : closeP]
			fmt.Fprintf(&b, "<%s|%s>", url, text)
			i = closeP + 1
		} else {
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String()
}
