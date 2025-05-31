package reminder

import (
	"context"
	"fmt"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/go-telegram/fsm"
	"strconv"
	"strings"
	"time"
)

const (
	ReminderTimeFormat = "02-01-2006 15:04"

	stateDefault     fsm.StateID = "default"
	stateAskReminder fsm.StateID = "ask_name"
	stateAskTime     fsm.StateID = "ask_age"
	stateFinish      fsm.StateID = "finish"

	INVALID_TIME_FORMAT_MSG = "Invalid time format. Please use: " + ReminderTimeFormat

	HELP_MSG = `
Commands:
	/setreminder - to set a reminder with a specific time
	/reminders - to get reminders
`

	SET_REMINDER_COMMAND  = "/setreminder"
	GET_REMINDERS_COMMAND = "/reminders"
)

type (
	Bot struct {
		BaseBot
		db *ReminderDB
	}
	ContextStruct struct {
		ctx    context.Context
		userId TGUserId
		chatId TGChatId
	}
)

var (
	fsmStates *fsm.FSM
)

func NewBot(token string) *Bot {
	dbConn, err := connectDB()

	if err != nil {
		panic(err)
	}

	reminderBot := &Bot{
		BaseBot: BaseBot{},
		db:      NewReminderDB(dbConn),
	}

	opts := []bot.Option{
		bot.WithDefaultHandler(reminderBot.HandleDefault),
		bot.WithMessageTextHandler(SET_REMINDER_COMMAND, bot.MatchTypeExact, reminderBot.HandleSetReminder),
		bot.WithMessageTextHandler(GET_REMINDERS_COMMAND, bot.MatchTypeExact, reminderBot.HandleGetReminders),
		bot.WithCallbackQueryDataHandler("remove_reminder_", bot.MatchTypePrefix, reminderBot.RemoveReminder),
	}

	baseBot, err := bot.New(token, opts...)

	reminderBot.BaseBot.Bot = baseBot

	if err != nil {
		panic(err)
	}

	fsmStates = fsm.New(stateDefault, map[fsm.StateID]fsm.Callback{
		stateAskReminder: reminderBot.callbackSetReminder,
		stateAskTime:     reminderBot.callbackSetTime,
		stateFinish:      reminderBot.callbackFinish,
	})

	return reminderBot
}

func (reminderBot *Bot) Start(ctx context.Context) {
	if reminderBot.Bot == nil {
		fmt.Println("Bot is not initialized")
		return
	}

	fmt.Println("Starting reminder bot...")

	go reminderBot.FetchNotificationsAndNotify()

	// Start the bot
	defer (func() {
		reminderBot.Bot.Start(ctx)
	})()
}

func (reminderBot *Bot) Close(ctx context.Context) {
	if reminderBot.db != nil {
		reminderBot.db.CloseConn()
	}

	if reminderBot.Bot != nil {
		_, err := reminderBot.Bot.Close(ctx)
		if err != nil {
			fmt.Println("Error closing bot: ", err)
		}
	}
}

func (reminderBot *Bot) FetchNotificationsAndNotify() {
	for {
		time.Sleep(time.Second * 5)

		reminders, err := reminderBot.db.getClosestReminders()
		if err != nil {
			fmt.Println("Error getting reminders: ", err)
			return
		}

		for _, r := range reminders {
			if time.Now().After(r.Time) {
				reminderBot.sendMessage(context.Background(), &bot.SendMessageParams{
					ChatID: r.ChatId,
					Text:   "Reminder: " + r.Message,
				})
				reminderBot.db.deleteReminderFromDbById(r.Id)
			}
		}
	}
}

func (reminderBot *Bot) HandleSetReminder(ctx context.Context, _ *bot.Bot, update *models.Update) {
	userId := TGUserId(update.Message.From.ID)
	chatId := TGChatId(update.Message.Chat.ID)

	if update.Message == nil {
		return
	}

	currentState := fsmStates.Current(int64(userId))
	if currentState != stateDefault {
		return
	}

	fsmStates.Set(int64(userId), "msgId", update.Message.ID)

	fsmStates.Transition(int64(userId), stateAskReminder, ContextStruct{
		ctx:    ctx,
		userId: userId,
		chatId: chatId,
	})
}

func (reminderBot *Bot) HandleGetReminders(ctx context.Context, _ *bot.Bot, update *models.Update) {
	chatId := TGChatId(update.Message.Chat.ID)

	reminderBot.sendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatId,
		Text:   "Getting reminders...",
	})

	userId := TGUserId(update.Message.From.ID)
	reminders, err := reminderBot.db.getRemindersFromDbByUserId(userId)
	if err != nil {
		reminderBot.sendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatId,
			Text:   "Error getting reminders: " + err.Error(),
		})
		return
	}

	if len(reminders) == 0 {
		reminderBot.sendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatId,
			Text:   "No reminders found",
		})
		return
	}

	reminderBot.sendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatId,
		Text:   "Reminders found",
	})
	for _, r := range reminders {
		kb := &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{Text: "Remove", CallbackData: "remove_reminder_" + strconv.FormatInt(r.Id, 10)},
				},
			},
		}

		reminderBot.sendMessage(ctx, &bot.SendMessageParams{
			ChatID:      chatId,
			Text:        r.String(),
			ReplyMarkup: kb,
		})
	}
}

func (reminderBot *Bot) RemoveReminder(ctx context.Context, _ *bot.Bot, update *models.Update) {
	chatId := TGChatId(update.CallbackQuery.Message.Message.Chat.ID)

	if update.CallbackQuery == nil {
		return
	}

	reminderIdStr, _ := strings.CutPrefix(update.CallbackQuery.Data, "remove_reminder_")

	reminderId, err := strconv.ParseInt(reminderIdStr, 10, 64)
	if err != nil {
		reminderBot.sendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatId,
			Text:   "Error parsing reminder ID: " + err.Error(),
		})
		return
	}

	err = reminderBot.db.deleteReminderFromDbById(reminderId)
	if err != nil {
		reminderBot.sendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatId,
			Text:   "Error deleting reminder: " + err.Error(),
		})
		return
	}

	reminderBot.sendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatId,
		Text:   "Reminder deleted",
	})
}

func (reminderBot *Bot) callbackSetReminder(_ *fsm.FSM, args ...any) {
	currentContext := args[0].(ContextStruct)

	reminderBot.sendMessage(currentContext.ctx, &bot.SendMessageParams{
		ChatID: currentContext.chatId,
		Text:   "Set reminder text please",
	})
}

func (reminderBot *Bot) callbackSetTime(f *fsm.FSM, args ...any) {
	currentContext := args[0].(ContextStruct)

	reminderBot.sendMessage(currentContext.ctx, &bot.SendMessageParams{
		ChatID: currentContext.chatId,
		Text:   "Set reminder time please",
	})
}

func (reminderBot *Bot) callbackFinish(f *fsm.FSM, args ...any) {
	currentContext := args[0].(ContextStruct)

	reminderMsg, _ := fsmStates.Get(int64(currentContext.userId), "reminder")
	reminderTime, _ := fsmStates.Get(int64(currentContext.userId), "time")
	msgIdStr, _ := fsmStates.Get(int64(currentContext.userId), "msgId")

	dbErr := reminderBot.db.saveReminderToDB(Reminder{
		ChatId:  currentContext.chatId,
		UserId:  currentContext.userId,
		Message: reminderMsg.(string),
		Time:    reminderTime.(time.Time),
	})

	if dbErr != nil {
		reminderBot.sendMessage(currentContext.ctx, &bot.SendMessageParams{
			ChatID: currentContext.chatId,
			Text:   "Error saving reminder to database: " + dbErr.Error(),
		})
		return
	}

	msgId, ok := msgIdStr.(int)

	reminderBot.sendMessage(currentContext.ctx, &bot.SendMessageParams{
		ChatID: currentContext.chatId,
		Text:   "Reminder set!\nReminder: " + reminderMsg.(string) + "\nTime: " + reminderTime.(time.Time).Add(3*time.Hour).Format(ReminderTimeFormat),
	})
	if ok {
		reminderBot.setReaction(currentContext.ctx, &models.Update{
			Message: &models.Message{
				ID: msgId,
				Chat: models.Chat{
					ID: int64(currentContext.chatId),
				},
			},
		}, "🗿")
	}

	fsmStates.Transition(int64(currentContext.userId), stateDefault)
}

func (reminderBot *Bot) HandleDefault(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	userID := update.Message.From.ID
	chatID := update.Message.Chat.ID

	currentState := fsmStates.Current(userID)

	switch currentState {
	case stateDefault:
		sendHelp(ctx, b, update)
		return

	case stateAskReminder:
		fsmStates.Set(userID, "reminder", update.Message.Text)

		fsmStates.Transition(userID, stateAskTime, ContextStruct{
			ctx:    ctx,
			userId: TGUserId(userID),
			chatId: TGChatId(chatID),
		})
	case stateAskTime:
		reminderTime, err := parseTime(update.Message.Text)

		if err != nil {
			reminderBot.sendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   INVALID_TIME_FORMAT_MSG + ". Error: " + err.Error(),
			})
			return
		}

		reminderTime = reminderTime.Add(-time.Hour * 3) // Add 3 hours to the time

		fsmStates.Set(userID, "time", reminderTime)

		fsmStates.Transition(userID, stateFinish, ContextStruct{
			ctx:    ctx,
			userId: TGUserId(userID),
			chatId: TGChatId(chatID),
		})

	case stateFinish:
		fsmStates.Transition(userID, stateDefault)
	}

}

func sendHelp(ctx context.Context, b *bot.Bot, update *models.Update) error {
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   HELP_MSG,
	})

	if err != nil {
		fmt.Println("send help err: ", err)
	}

	return err
}

func parseTime(timeStr string) (time.Time, error) {
	parsedTime, err := time.Parse(ReminderTimeFormat, timeStr)
	if err != nil {
		return time.Time{}, err
	}
	return parsedTime, nil
}
