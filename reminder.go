package reminder

import (
	"fmt"
	"slices"
	"time"
)

type (
	TGUserId int64
	TGChatId int64

	Reminder struct {
		Id      int64
		ChatId  TGChatId
		UserId  TGUserId
		Message string
		Time    time.Time
	}

	Reminders []Reminder

	UserReminders map[TGUserId]Reminders
)

func (ur UserReminders) AddReminder(reminder Reminder) error {
	if _, ok := ur[reminder.UserId]; !ok {
		ur[reminder.UserId] = Reminders{}
	}

	ur[reminder.UserId] = append(ur[reminder.UserId], reminder)

	return nil
}

func (ur UserReminders) RemoveReminder(userId TGUserId, reminderIndex int) error {
	if _, ok := ur[userId]; !ok {
		return nil
	}

	if reminderIndex < 0 || reminderIndex >= len(ur[userId]) {
		return nil
	}

	ur[userId] = slices.Delete(ur[userId], reminderIndex, reminderIndex+1)

	return nil
}

func (ur UserReminders) GetUserReminders(id TGUserId) Reminders {
	if reminders, ok := ur[id]; ok {
		return reminders
	}

	return Reminders{}
}

func (r *Reminder) String() string {
	localTime := time.Now()

	timeDiff := r.Time.Local().Sub(localTime)

	days := timeDiff / (24 * time.Hour)
	hours := (timeDiff % (24 * time.Hour)) / time.Hour
	minutes := (timeDiff % (24 * time.Hour) % time.Hour) / time.Minute
	return fmt.Sprintf("Message: %s, Time to reminder: %s", r.Message, fmt.Sprintf("%02d days %02d hours %02d minutes", days, hours, minutes))
}
