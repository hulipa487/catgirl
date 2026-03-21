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
	bot       *tgbotapi.BotAPI
	config    *config.TelegramConfig
	repo      *repository.Repository
	sessionSvc TelegramSessionService
	logger    zerolog.Logger
}

type TelegramSessionService interface {
	GetSessionByTelegramUser(ctx context.Context, telegramUserID int64) (*TelegramSession, error)
	CreateSession(ctx context.Context, telegramUserID int64, username, firstName, lastName string) (*TelegramSession, error)
}

type TelegramSession struct {
	ID               interface{}
	TelegramUserID   int64
	Name             string
}

func NewTelegramService(cfg *config.TelegramConfig, repo *repository.Repository, sessionSvc TelegramSessionService, logger zerolog.Logger) (*TelegramService, error) {
	bot, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create Telegram bot: %w", err)
	}

	bot.Debug = false

	svc := &TelegramService{
		bot:       bot,
		config:    cfg,
		repo:      repo,
		sessionSvc: sessionSvc,
		logger:    logger,
	}

	return svc, nil
}

func (s *TelegramService) SetWebhook(ctx context.Context) error {
	wh, err := tgbotapi.NewWebhook(s.config.WebhookURL)
	if err != nil {
		return fmt.Errorf("failed to create webhook config: %w", err)
	}

	_, err = s.bot.Request(wh)
	if err != nil {
		return fmt.Errorf("failed to set webhook: %w", err)
	}

	s.logger.Info().
		Str("webhook_url", s.config.WebhookURL).
		Msg("webhook set")

	return nil
}

func (s *TelegramService) HandleUpdate(update *tgbotapi.Update) error {
	if update.Message == nil {
		return nil
	}

	if update.Message.IsCommand() {
		return s.handleCommand(update.Message)
	}

	return s.handleMessage(update.Message)
}

func (s *TelegramService) handleCommand(msg *tgbotapi.Message) error {
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

func (s *TelegramService) handleMessage(msg *tgbotapi.Message) error {
	ctx := context.Background()

	isBanned, err := s.repo.IsTelegramUserBanned(ctx, msg.From.ID)
	if err != nil {
		s.logger.Error().Err(err).Int64("user_id", msg.From.ID).Msg("failed to check ban status")
	}
	if isBanned {
		return s.sendReply(msg, "You are not allowed to use this bot.")
	}

	session, err := s.sessionSvc.GetSessionByTelegramUser(ctx, msg.From.ID)
	if err != nil {
		s.logger.Error().Err(err).Int64("user_id", msg.From.ID).Msg("failed to get session")
		return s.sendReply(msg, "An error occurred. Please try again later.")
	}

	if session == nil {
		session, err = s.sessionSvc.CreateSession(ctx, msg.From.ID,
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
		Str("session_id", fmt.Sprintf("%v", session.ID)).
		Str("message", truncate(userMessage, 100)).
		Msg("user message received")

	return s.sendReply(msg, fmt.Sprintf("Message received: %s", userMessage))
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

func (s *TelegramService) sendReply(msg *tgbotapi.Message, text string) error {
	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ReplyToMessageID = msg.MessageID

	_, err := s.bot.Send(reply)
	return err
}

func (s *TelegramService) SendMessage(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	_, err := s.bot.Send(msg)
	return err
}

func (s *TelegramService) GetBotInfo() (tgbotapi.User, error) {
	return s.bot.Self, nil
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
