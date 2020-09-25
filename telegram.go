package fiohealth

import (
	"errors"
	"fmt"
	tg "github.com/go-telegram-bot-api/telegram-bot-api"
	"log"
	"strings"
)

func Notify(alert string, apiKey string, channel string, baseUrl string) error {
	bot, err := tg.NewBotAPI(apiKey)
	if err != nil {
		log.Println(err)
		return err
	}

	split := strings.Split(alert, ": ")
	if len(split) < 2 {
		return errors.New("invalid alert message format")
	}
	mc := tg.NewMessageToChannel(channel, fmt.Sprintf(`<b><a href="%s">%s</a></b>: %s`, baseUrl, split[0], strings.Join(split[1:], ": ")))
	mc.ParseMode = "html"
	_, err = bot.Send(mc)
	if err != nil {
		log.Println(err)
	}
	return nil
}
