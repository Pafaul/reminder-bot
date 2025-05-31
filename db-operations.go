package reminder

import "database/sql"

type (
	ReminderDB struct {
		db *sql.DB
	}
)

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

func NewReminderDB(dbConn *sql.DB) *ReminderDB {
	return &ReminderDB{db: dbConn}
}

func (rDb *ReminderDB) CloseConn() {
	if rDb.db != nil {
		err := rDb.db.Close()
		if err != nil {
			// Handle error if needed
			return
		}
	}
}

// generate a function to save a reminder to the database, which accepts db connection and reminder object
func (rDb *ReminderDB) saveReminderToDB(r Reminder) error {
	_, err := rDb.db.Exec("INSERT INTO reminders (user_id, chat_id, message, time) VALUES (?, ?, ?, ?)", r.UserId, r.ChatId, r.Message, r.Time)
	if err != nil {
		return err
	}
	return nil
}

func (rDb *ReminderDB) getRemindersFromDbByUserId(userId TGUserId) (Reminders, error) {
	rows, err := rDb.db.Query("SELECT id, user_id, chat_id, message, time FROM reminders WHERE user_id = ?", userId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reminders Reminders
	for rows.Next() {
		var r Reminder
		err := rows.Scan(&r.Id, &r.UserId, &r.ChatId, &r.Message, &r.Time)
		if err != nil {
			return nil, err
		}
		reminders = append(reminders, r)
	}

	return reminders, nil
}

func (rDb *ReminderDB) deleteReminderFromDbById(reminderId int64) error {
	_, err := rDb.db.Exec("DELETE FROM reminders WHERE id = ?", reminderId)
	if err != nil {
		return err
	}
	return nil
}

func (rDb *ReminderDB) getClosestReminders() (Reminders, error) {
	rows, err := rDb.db.Query("SELECT id, user_id, chat_id, message, time FROM reminders ORDER BY time")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reminders Reminders
	for rows.Next() {
		var r Reminder
		err := rows.Scan(&r.Id, &r.UserId, &r.ChatId, &r.Message, &r.Time)
		if err != nil {
			return nil, err
		}
		reminders = append(reminders, r)
	}

	return reminders, nil
}
