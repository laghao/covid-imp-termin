package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/eleboucher/covid/models/chat"
	"github.com/eleboucher/covid/vaccines"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	log "github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

const (
	startButton      = "Start"
	stopButton       = "Stop"
	filterButton     = "Add filters (multiple choices available)"
	azButton         = "Look for AstraZeneca"
	jjButton         = "Look for Johnson & Johnson"
	vcButton         = "Look for MRNA vaccine (clinics and vaccination centers)"
	everythingButton = "Look for everything"
	contributeButton = "Contribute and support"
	infoFilterButton = "Info about filters"
	backButton       = "Back"
)

var keyboard = tgbotapi.NewReplyKeyboard(
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton(startButton),
		tgbotapi.NewKeyboardButton(stopButton),
	),
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton(filterButton),
	),
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton(contributeButton),
	),
)

var filtersKeyboard = tgbotapi.NewReplyKeyboard(
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton(azButton),
		tgbotapi.NewKeyboardButton(jjButton),
	),
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton(vcButton),
	),
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton(everythingButton),
	),
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton(infoFilterButton),
		tgbotapi.NewKeyboardButton(backButton),
	),
)

// Telegram Holds the structure for the telegram bot
type Telegram struct {
	bot       *tgbotapi.BotAPI
	limiter   *rate.Limiter
	channel   int64
	chatModel *chat.Model
}

// NewBot return a new Telegram Bot
func NewBot(bot *tgbotapi.BotAPI, chatModel *chat.Model) *Telegram {
	return &Telegram{
		bot:       bot,
		chatModel: chatModel,
		limiter:   rate.NewLimiter(rate.Every(time.Second/30), 1),
	}
}

// SendMessage send a message in string to a channel id
func (t *Telegram) SendMessage(message string, channel int64) error {
	msg := tgbotapi.MessageConfig{
		BaseChat: tgbotapi.BaseChat{
			ChatID:           channel,
			ReplyToMessageID: 0,
		},
		Text:                  message,
		DisableWebPagePreview: true,
	}
	ctx := context.Background()
	err := t.limiter.Wait(ctx)
	if err != nil {
		return err
	}
	_, err = t.bot.Send(msg)
	if err != nil {
		if err.Error() == "Forbidden: bot was blocked by the user" || err.Error() == "Forbidden: user is deactivated" {
			_, err := t.chatModel.Delete(channel)
			if err != nil {
				return err
			}
		}
		return err
	}
	return nil
}

// SendMessageToAllUser send a message to all the enabled users
func (t *Telegram) SendMessageToAllUser(result *vaccines.Result) error {
	chats, err := t.chatModel.List(&result.VaccineName)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup

	wg.Add(len(chats))
	log.Infof("sending message %s for %d users\n", result.Message, len(chats))

	for _, chat := range chats {
		chat := chat
		go func() {
			defer wg.Done()
			chat := chat
			err := t.SendMessage(result.Message, chat.ID)
			if err != nil {
				log.Error(err)
			}
		}()
	}
	wg.Wait()
	return nil
}

// HandleNewUsers handle the commands from telegrams
func (t *Telegram) HandleNewUsers() error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := t.bot.GetUpdatesChan(u)
	if err != nil {
		return err
	}

	for update := range updates {
		update := update
		go func() {
			if update.Message == nil { // ignore any non-Message Updates
				return
			}

			msg := tgbotapi.NewMessage(update.Message.Chat.ID, update.Message.Text)
			switch update.Message.Text {
			case "open", backButton:
				msg.ReplyMarkup = keyboard
				t.bot.Send(msg)
			case "close":
				msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
				t.bot.Send(msg)
			case contributeButton:
				err = t.SendMessage("Welcome to covid bot", update.Message.Chat.ID)
				if err != nil {
					log.Error(err)
				}
			case filterButton:
				msg.ReplyMarkup = filtersKeyboard
				t.bot.Send(msg)
			case stopButton:
				err := t.stopChat(update.Message.Chat.ID)
				if err != nil {
					log.Error(err)
				}
			case startButton:
				err := t.startChat(update.Message.Chat.ID)
				if err != nil {
					log.Error(err)
				}
			case azButton:
				_, err := t.chatModel.UpdateFilters(update.Message.Chat.ID, vaccines.AstraZeneca)
				if err != nil {
					log.Error(err)
					return
				}
				err = t.SendMessage("subscribed to AstraZeneca updates", update.Message.Chat.ID)
				if err != nil {
					log.Error(err)
					return
				}
			case jjButton:
				_, err := t.chatModel.UpdateFilters(update.Message.Chat.ID, vaccines.JohnsonAndJohnson)
				if err != nil {
					log.Error(err)
					return
				}
				err = t.SendMessage("subscribed to Johnson And Johnson updates", update.Message.Chat.ID)
				if err != nil {
					log.Error(err)
					return
				}
			case vcButton:
				_, err := t.chatModel.UpdateFilters(update.Message.Chat.ID, vaccines.MRNA)
				if err != nil {
					log.Error(err)
					return
				}
				err = t.SendMessage("subscribed to MRNA vaccines updates", update.Message.Chat.ID)
				if err != nil {
					log.Error(err)
					return
				}
			case everythingButton:
				_, err := t.chatModel.UpdateFilters(update.Message.Chat.ID, "")
				if err != nil {
					log.Error(err)
					return
				}
				err = t.SendMessage("subscribed to every updates", update.Message.Chat.ID)
				if err != nil {
					log.Error(err)
					return
				}
			case infoFilterButton:
				chat, err := t.chatModel.Find(update.Message.Chat.ID)
				if err != nil {
					log.Error(err)
					return
				}
				var filters string
				if len(chat.Filters) == 0 {
					filters = "unfiltered"
				} else {
					filters = strings.Join(chat.Filters, "\n")
				}
				msg := fmt.Sprintf("your current filters are :\n%s\n\nSelect %s to reset them", filters, everythingButton)
				err = t.SendMessage(msg, update.Message.Chat.ID)
				if err != nil {
					log.Error(err)
					return
				}
			}

			switch update.Message.Command() {
			case "start":
				err := t.startChat(update.Message.Chat.ID)
				if err != nil {
					log.Error(err)
				}
			case "stop":
				err := t.stopChat(update.Message.Chat.ID)
				if err != nil {
					log.Error(err)
				}
			case "open":
				msg.ReplyMarkup = filtersKeyboard
				t.bot.Send(msg)
			case "contribute":
				err = t.SendMessage("Welcome to covid bot", update.Message.Chat.ID)
				if err != nil {
					log.Error(err)
				}
			}
		}()
	}
	log.Info("done with telegram handler")
	return nil
}

func (t *Telegram) startChat(chatID int64) error {
	log.Infof("adding chat %d\n", chatID)

	_, err := t.chatModel.Create(chatID)
	if err != nil {
		if errors.Is(err, chat.ErrChatAlreadyExist) {
			_, err := t.chatModel.Enable(chatID)
			if err != nil {
				return err
			}
			err = t.SendMessage(`
			Welcome to covid bot`, chatID)
			if err != nil {
				return err
			}
			return err
		}
		return err
	}
	err = t.SendMessage(`
	Welcome to covid bot`, chatID)
	if err != nil {
		return err
	}
	return nil
}

func (t *Telegram) stopChat(chatID int64) error {
	log.Infof("removing chat %d\n", chatID)

	_, err := t.chatModel.Delete(chatID)
	if err != nil {
		return err
	}
	err = t.SendMessage(`
	Welcome to covid bot`, chatID)
	if err != nil {
		return err
	}
	return nil
}
