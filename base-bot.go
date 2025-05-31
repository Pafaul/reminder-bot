package reminder

import (
	"context"
	"fmt"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type (
	BaseBot struct {
		*bot.Bot
	}
)

func (b *BaseBot) sendMessage(ctx context.Context, msg *bot.SendMessageParams) error {
	_, err := b.SendMessage(ctx, msg)

	if err != nil {
		fmt.Println("err: ", err)
		return err
	}
	return nil
}

func (b *BaseBot) setReaction(ctx context.Context, update *models.Update, reaction string) error {
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
