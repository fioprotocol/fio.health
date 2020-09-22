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

	//// clear out any pending messages, we never respond.
	//u := tg.NewUpdate(0)
	//u.Timeout = 2
	//updates, err := bot.GetUpdatesChan(u)
	//if err != nil {
	//	log.Println(err)
	//	return err
	//}
	//updates.Clear()

	split := strings.Split(alert, ": ")
	if len(split) < 2 {
		return errors.New("invalid alert message format")
	}
	mc := tg.NewMessageToChannel(channel, fmt.Sprintf(`<b><a href="%s">%s</a></b>: %s`, baseUrl, split[0], strings.Join(split[1:], ": ")))
	mc.ParseMode = "html"
	m, err := bot.Send(mc)
	if err != nil {
		log.Println(err)
	}
	log.Printf("%+v\n", m)
	return nil
}
