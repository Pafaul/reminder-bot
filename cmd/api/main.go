package main

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/go-telegram/fsm"
	_ "github.com/mattn/go-sqlite3"
	"os"
	"os/signal"
	"pafaul/reminder"
	_ "pafaul/reminder/config"
	"strconv"
	"strings"
	"time"
)

// Send any text message to the bot after the bot has been started

const (
	ReminderTimeFormat = "02-01-2006 15:04"

	SET_REMINDER_COMMAND  = "/setreminder"
	GET_REMINDERS_COMMAND = "/reminders"

	stateDefault     fsm.StateID = "default"
	stateAskReminder fsm.StateID = "ask_name"
	stateAskTime     fsm.StateID = "ask_age"
	stateFinish      fsm.StateID = "finish"

	INVALID_REMINDER_FORMAT_MSG = "Invalid reminder format. Please use: /setreminder message # time"
	INVALID_TIME_FORMAT_MSG     = "Invalid time format. Please use: " + ReminderTimeFormat
)

type (
	Application struct {
		bot       *bot.Bot
		f         *fsm.FSM
		reminders reminder.UserReminders
		dbConn    *sql.DB
	}

	ContextStruct struct {
		ctx    context.Context
		userId reminder.TGUserId
		chatId reminder.TGChatId
	}
)

const (
	HELP_MSG = `
Commands:
	/setreminder - to set a reminder with a specific time
	/reminders - to get reminders
`
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	dbConn, dbErr := connectDB()
	if dbErr != nil {
		fmt.Println("Error connecting to the database: ", dbErr)
		return
	}
	defer dbConn.Close()

	app := &Application{}
	app.dbConn = dbConn

	opts := []bot.Option{
		bot.WithDefaultHandler(app.handleDefault),
		bot.WithMessageTextHandler(SET_REMINDER_COMMAND, bot.MatchTypeExact, app.handleSetReminder),
		bot.WithMessageTextHandler(GET_REMINDERS_COMMAND, bot.MatchTypeExact, app.handleGetReminders),
		bot.WithCallbackQueryDataHandler("remove_reminder_", bot.MatchTypePrefix, app.removeReminder),
	}

	app.f = fsm.New(stateDefault, map[fsm.StateID]fsm.Callback{
		stateAskReminder: app.callbackSetReminder,
		stateAskTime:     app.callbackSetTime,
		stateFinish:      app.callbackFinish,
	})

	b, err := bot.New(os.Getenv("TG_TOKEN"), opts...)
	if nil != err {
		// panics for the sake of simplicity.
		// you should handle this error properly in your code.
		panic(err)
	}

	app.bot = b
	app.reminders = reminder.UserReminders{}

	_, err = b.SetChatMenuButton(ctx, &bot.SetChatMenuButtonParams{
		ChatID: nil,
		MenuButton: &models.MenuButtonCommands{
			Type: "commands",
		},
	})

	go app.fetchNotificationsAndNotify()

	b.Start(ctx)
}

func (app *Application) fetchNotificationsAndNotify() {
	for {
		time.Sleep(time.Second * 5)

		reminders, err := getRemindersFromDbSortedByTime(app.dbConn)
		if err != nil {
			fmt.Println("Error getting reminders: ", err)
			return
		}

		for _, r := range reminders {
			if time.Now().After(r.Time) {
				sendMessage(context.Background(), app.bot, &bot.SendMessageParams{
					ChatID: r.ChatId,
					Text:   "Reminder: " + r.Message,
				})
				deleteReminderFromDbById(app.dbConn, r.Id)
			}
		}
	}
}

func (app *Application) handleSetReminder(ctx context.Context, b *bot.Bot, update *models.Update) {
	userId := reminder.TGUserId(update.Message.From.ID)
	chatId := reminder.TGChatId(update.Message.Chat.ID)

	if update.Message == nil {
		return
	}

	currentState := app.f.Current(int64(userId))
	if currentState != stateDefault {
		return
	}

	app.f.Set(int64(userId), "msgId", update.Message.ID)

	app.f.Transition(int64(userId), stateAskReminder, ContextStruct{
		ctx:    ctx,
		userId: userId,
		chatId: chatId,
	})
}

func (app *Application) handleGetReminders(ctx context.Context, b *bot.Bot, update *models.Update) {
	chatId := reminder.TGChatId(update.Message.Chat.ID)

	sendMessage(ctx, b, &bot.SendMessageParams{
		ChatID: chatId,
		Text:   "Getting reminders...",
	})

	userId := reminder.TGUserId(update.Message.From.ID)
	reminders, err := getRemindersFromDbByUserId(app.dbConn, userId)
	if err != nil {
		sendMessage(ctx, b, &bot.SendMessageParams{
			ChatID: chatId,
			Text:   "Error getting reminders: " + err.Error(),
		})
		return
	}

	if len(reminders) == 0 {
		sendMessage(ctx, b, &bot.SendMessageParams{
			ChatID: chatId,
			Text:   "No reminders found",
		})
		return
	}

	sendMessage(ctx, b, &bot.SendMessageParams{
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

		sendMessage(ctx, b, &bot.SendMessageParams{
			ChatID:      chatId,
			Text:        r.String(),
			ReplyMarkup: kb,
		})
	}
}

func (app *Application) removeReminder(ctx context.Context, b *bot.Bot, update *models.Update) {
	chatId := reminder.TGChatId(update.CallbackQuery.Message.Message.Chat.ID)

	if update.CallbackQuery == nil {
		return
	}

	reminderIdStr, _ := strings.CutPrefix(update.CallbackQuery.Data, "remove_reminder_")

	reminderId, err := strconv.ParseInt(reminderIdStr, 10, 64)
	if err != nil {
		sendMessage(ctx, b, &bot.SendMessageParams{
			ChatID: chatId,
			Text:   "Error parsing reminder ID: " + err.Error(),
		})
		return
	}

	err = deleteReminderFromDbById(app.dbConn, reminderId)
	if err != nil {
		sendMessage(ctx, b, &bot.SendMessageParams{
			ChatID: chatId,
			Text:   "Error deleting reminder: " + err.Error(),
		})
		return
	}

	sendMessage(ctx, b, &bot.SendMessageParams{
		ChatID: chatId,
		Text:   "Reminder deleted",
	})
}

func (app *Application) callbackSetReminder(f *fsm.FSM, args ...any) {
	currentContext := args[0].(ContextStruct)

	sendMessage(currentContext.ctx, app.bot, &bot.SendMessageParams{
		ChatID: currentContext.chatId,
		Text:   "Set reminder text please",
	})
}

func (app *Application) callbackSetTime(f *fsm.FSM, args ...any) {
	currentContext := args[0].(ContextStruct)

	sendMessage(currentContext.ctx, app.bot, &bot.SendMessageParams{
		ChatID: currentContext.chatId,
		Text:   "Set reminder time please",
	})
}

func (app *Application) callbackFinish(f *fsm.FSM, args ...any) {
	currentContext := args[0].(ContextStruct)

	reminderMsg, _ := app.f.Get(int64(currentContext.userId), "reminder")
	reminderTime, _ := app.f.Get(int64(currentContext.userId), "time")
	msgIdStr, _ := app.f.Get(int64(currentContext.userId), "msgId")

	dbErr := saveReminderToDB(app.dbConn, reminder.Reminder{
		ChatId:  currentContext.chatId,
		UserId:  currentContext.userId,
		Message: reminderMsg.(string),
		Time:    reminderTime.(time.Time),
	})

	if dbErr != nil {
		sendMessage(currentContext.ctx, app.bot, &bot.SendMessageParams{
			ChatID: currentContext.chatId,
			Text:   "Error saving reminder to database: " + dbErr.Error(),
		})
		return
	}

	msgId, ok := msgIdStr.(int)

	app.bot.SendMessage(currentContext.ctx, &bot.SendMessageParams{
		ChatID: currentContext.chatId,
		Text:   "Reminder set!\nReminder: " + reminderMsg.(string) + "\nTime: " + reminderTime.(time.Time).Add(3*time.Hour).Format(ReminderTimeFormat),
	})
	if ok {
		setReaction(currentContext.ctx, app.bot, &models.Update{
			Message: &models.Message{
				ID: msgId,
				Chat: models.Chat{
					ID: int64(currentContext.chatId),
				},
			},
		}, "🗿")
	}

	app.f.Transition(int64(currentContext.userId), stateDefault)
}

func (app *Application) handleDefault(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	userID := update.Message.From.ID
	chatID := update.Message.Chat.ID

	currentState := app.f.Current(userID)

	switch currentState {
	case stateDefault:
		sendHelp(ctx, b, update)
		return

	case stateAskReminder:
		app.f.Set(userID, "reminder", update.Message.Text)

		app.f.Transition(userID, stateAskTime, ContextStruct{
			ctx:    ctx,
			userId: reminder.TGUserId(userID),
			chatId: reminder.TGChatId(chatID),
		})
	case stateAskTime:
		reminderTime, err := parseTime(update.Message.Text)

		if err != nil {
			sendMessage(ctx, b, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   INVALID_TIME_FORMAT_MSG + ". Error: " + err.Error(),
			})
			return
		}

		reminderTime = reminderTime.Add(-time.Hour * 3) // Add 3 hours to the time

		app.f.Set(userID, "time", reminderTime)

		app.f.Transition(userID, stateFinish, ContextStruct{
			ctx:    ctx,
			userId: reminder.TGUserId(userID),
			chatId: reminder.TGChatId(chatID),
		})

	case stateFinish:
		app.f.Transition(userID, stateDefault)
	}

}

func parseTime(timeStr string) (time.Time, error) {
	parsedTime, err := time.Parse(ReminderTimeFormat, timeStr)
	if err != nil {
		return time.Time{}, err
	}
	return parsedTime, nil
}

func sendMessage(ctx context.Context, b *bot.Bot, msg *bot.SendMessageParams) error {
	_, err := b.SendMessage(ctx, msg)

	if err != nil {
		fmt.Println("err: ", err)
		return err
	}
	return nil
}

func setReaction(ctx context.Context, b *bot.Bot, update *models.Update, reaction string) error {
	_, err := b.SetMessageReaction(ctx, &bot.SetMessageReactionParams{
		ChatID:    update.Message.Chat.ID,
		MessageID: update.Message.ID,
		Reaction: []models.ReactionType{
			{
				Type:              models.ReactionTypeTypeEmoji,
				ReactionTypeEmoji: &models.ReactionTypeEmoji{Type: models.ReactionTypeTypeEmoji, Emoji: reaction},
			},
		},
	})
	if err != nil {
		fmt.Println("Error setting reaction: ", err)
		return err
	}
	return nil
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

func connectDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "./reminders.db")
	if err != nil {
		return nil, err
	}

	err = db.Ping()
	if err != nil {
		return nil, err
	}

	_, err = db.Exec("CREATE TABLE IF NOT EXISTS reminders (id INTEGER PRIMARY KEY, user_id INTEGER, chat_id INTEGER, message TEXT, time DATETIME)")
	if err != nil {
		return nil, err
	}

	return db, nil
}

// generate a function to save a reminder to the database, which accepts db connection and reminder object
func saveReminderToDB(db *sql.DB, r reminder.Reminder) error {
	_, err := db.Exec("INSERT INTO reminders (user_id, chat_id, message, time) VALUES (?, ?, ?, ?)", r.UserId, r.ChatId, r.Message, r.Time)
	if err != nil {
		return err
	}
	return nil
}

func getRemindersFromDbByUserId(db *sql.DB, userId reminder.TGUserId) (reminder.Reminders, error) {
	rows, err := db.Query("SELECT id, user_id, chat_id, message, time FROM reminders WHERE user_id = ?", userId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reminders reminder.Reminders
	for rows.Next() {
		var r reminder.Reminder
		err := rows.Scan(&r.Id, &r.UserId, &r.ChatId, &r.Message, &r.Time)
		if err != nil {
			return nil, err
		}
		reminders = append(reminders, r)
	}

	return reminders, nil
}

func deleteReminderFromDbById(db *sql.DB, reminderId int64) error {
	_, err := db.Exec("DELETE FROM reminders WHERE id = ?", reminderId)
	if err != nil {
		return err
	}
	return nil
}

func getRemindersFromDbSortedByTime(db *sql.DB) (reminder.Reminders, error) {
	rows, err := db.Query("SELECT id, user_id, chat_id, message, time FROM reminders ORDER BY time")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reminders reminder.Reminders
	for rows.Next() {
		var r reminder.Reminder
		err := rows.Scan(&r.Id, &r.UserId, &r.ChatId, &r.Message, &r.Time)
		if err != nil {
			return nil, err
		}
		reminders = append(reminders, r)
	}

	return reminders, nil
}
