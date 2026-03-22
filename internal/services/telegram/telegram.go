package telegram

import (
	"context"
	"fmt"

	"github.com/hulipa487/catgirl/internal/config"
	"github.com/hulipa487/catgirl/internal/repository"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rs/zerolog"
)

type TelegramService struct {
	bots       []*tgbotapi.BotAPI
	config     *config.TelegramConfig
	repo       *repository.Repository
	sessionSvc TelegramSessionService
	logger     zerolog.Logger
}

type TelegramSessionService interface {
	GetSessionIDByTelegramUser(ctx context.Context, telegramUserID int64) (interface{}, error)
	CreateSessionForTelegramUser(ctx context.Context, telegramUserID int64, botToken string, username, firstName, lastName string) (interface{}, error)
	HandleUserMessage(ctx context.Context, sessionID interface{}, telegramUserID int64, message string) error
}

type TelegramSession struct {
	ID               interface{}
	TelegramUserID   int64
	Name             string
}

func NewTelegramService(cfg *config.TelegramConfig, repo *repository.Repository, sessionSvc TelegramSessionService, logger zerolog.Logger) (*TelegramService, error) {
	if len(cfg.Bots) == 0 {
		logger.Warn().Msg("Telegram Bots list is empty. Telegram integration will be disabled until configured.")
		return &TelegramService{
			config:     cfg,
			repo:       repo,
			sessionSvc: sessionSvc,
			logger:     logger,
		}, nil
	}

	var bots []*tgbotapi.BotAPI
	for _, bCfg := range cfg.Bots {
		if bCfg.BotToken == "" {
			continue
		}
		bot, err := tgbotapi.NewBotAPI(bCfg.BotToken)
		if err != nil {
			logger.Warn().Err(err).Msg("Failed to create Telegram bot instance. Skipping.")
			continue
		}
		bot.Debug = false
		bots = append(bots, bot)
	}

	svc := &TelegramService{
		bots:       bots,
		config:     cfg,
		repo:       repo,
		sessionSvc: sessionSvc,
		logger:     logger,
	}

	return svc, nil
}

func (s *TelegramService) SetWebhook(ctx context.Context) error {
	if len(s.bots) == 0 {
		return fmt.Errorf("no telegram bots initialized")
	}

	for i, bot := range s.bots {
		webhookURL := s.config.Bots[i].WebhookURL
		wh, err := tgbotapi.NewWebhook(webhookURL)
		if err != nil {
			s.logger.Error().Err(err).Msg("failed to create webhook config for bot")
			continue
		}

		_, err = bot.Request(wh)
		if err != nil {
			s.logger.Error().Err(err).Str("webhook_url", webhookURL).Msg("failed to set webhook for bot")
			continue
		}

		s.logger.Info().
			Str("webhook_url", webhookURL).
			Msg("webhook set successfully")
	}

	return nil
}

func (s *TelegramService) HandleUpdateForBot(update *tgbotapi.Update, botIndex int) error {
	if botIndex < 0 || botIndex >= len(s.bots) {
		return fmt.Errorf("invalid bot index")
	}

	if update.Message == nil {
		return nil
	}

	botConfig := s.config.Bots[botIndex]

	if update.Message.IsCommand() {
		return s.handleCommand(update.Message, botConfig)
	}

	return s.handleMessage(update.Message, botConfig)
}

func (s *TelegramService) handleCommand(msg *tgbotapi.Message, botConfig config.TelegramBotConfig) error {
	command := msg.Command()

	switch command {
	case "start":
		return s.handleStartCommand(msg)
	case "help":
		return s.handleHelpCommand(msg)
	case "status":
		return s.handleStatusCommand(msg)
	default:
		return s.sendReply(msg, "Unknown command. Type /help for available commands.")
	}
}

func (s *TelegramService) handleMessage(msg *tgbotapi.Message, botConfig config.TelegramBotConfig) error {
	ctx := context.Background()

	isBanned, err := s.repo.IsTelegramUserBanned(ctx, msg.From.ID)
	if err != nil {
		s.logger.Error().Err(err).Int64("user_id", msg.From.ID).Msg("failed to check ban status")
	}
	if isBanned {
		return s.sendReply(msg, "You are not allowed to use this bot.")
	}

	sessionID, err := s.sessionSvc.GetSessionIDByTelegramUser(ctx, msg.From.ID)
	if err != nil {
		s.logger.Error().Err(err).Int64("user_id", msg.From.ID).Msg("failed to get session")
		return s.sendReply(msg, "An error occurred. Please try again later.")
	}

	if sessionID == nil {
		sessionID, err = s.sessionSvc.CreateSessionForTelegramUser(ctx, msg.From.ID, botConfig.BotToken,
			ptrToString(msg.From.UserName),
			ptrToString(msg.From.FirstName),
			ptrToString(msg.From.LastName))
		if err != nil {
			s.logger.Error().Err(err).Int64("user_id", msg.From.ID).Msg("failed to create session")
			return s.sendReply(msg, "Failed to create session. Please try again.")
		}
	}

	userMessage := msg.Text

	s.logger.Info().
		Str("session_id", fmt.Sprintf("%v", sessionID)).
		Str("message", truncate(userMessage, 100)).
		Msg("user message received")

	if err := s.sessionSvc.HandleUserMessage(ctx, sessionID, msg.From.ID, userMessage); err != nil {
		s.logger.Error().Err(err).Str("session_id", fmt.Sprintf("%v", sessionID)).Msg("failed to handle user message")
		return s.sendReply(msg, "Sorry, I couldn't process your message.")
	}

	return nil
}

func (s *TelegramService) handleStartCommand(msg *tgbotapi.Message) error {
	welcome := "Welcome to Catgirl Agentic Runtime!\n\n"
	welcome += "I'm an AI agent that can help you with various tasks.\n"
	welcome += "\nCommands:\n"
	welcome += "/start - Start the bot\n"
	welcome += "/help - Show help\n"
	welcome += "/status - Check bot status\n"

	return s.sendReply(msg, welcome)
}

func (s *TelegramService) handleHelpCommand(msg *tgbotapi.Message) error {
	help := "Available commands:\n"
	help += "/start - Start the bot\n"
	help += "/help - Show this help\n"
	help += "/status - Check bot status\n"

	return s.sendReply(msg, help)
}

func (s *TelegramService) handleStatusCommand(msg *tgbotapi.Message) error {
	return s.sendReply(msg, "Bot is running and ready to help!")
}

func (s *TelegramService) getBotForChat(ctx context.Context, chatID int64) *tgbotapi.BotAPI {
	if len(s.bots) == 0 {
		return nil
	}

	session, err := s.repo.GetSessionByTelegramUser(ctx, chatID)
	if err == nil && session != nil {
		for i, cfg := range s.config.Bots {
			if cfg.BotToken == session.BotToken && i < len(s.bots) {
				return s.bots[i]
			}
		}
	}

	// Fallback to first bot
	return s.bots[0]
}

func (s *TelegramService) sendReply(msg *tgbotapi.Message, text string) error {
	bot := s.getBotForChat(context.Background(), msg.Chat.ID)
	if bot == nil {
		s.logger.Warn().Int64("chat_id", msg.Chat.ID).Msg("cannot send reply: no bot configured")
		return nil
	}
	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ReplyToMessageID = msg.MessageID

	_, err := bot.Send(reply)
	return err
}

func (s *TelegramService) SendMessage(chatID int64, text string) error {
	bot := s.getBotForChat(context.Background(), chatID)
	if bot == nil {
		s.logger.Warn().Int64("chat_id", chatID).Msg("cannot send message: no bot configured")
		return nil
	}
	msg := tgbotapi.NewMessage(chatID, text)
	_, err := bot.Send(msg)
	return err
}

func (s *TelegramService) GetBotInfo() (tgbotapi.User, error) {
	if len(s.bots) == 0 {
		return tgbotapi.User{}, fmt.Errorf("no bots configured")
	}
	return s.bots[0].Self, nil
}

func ptrToString(s string) string {
	return s
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
