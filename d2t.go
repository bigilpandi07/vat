package main

import (
	tbot "github.com/go-telegram-bot-api/telegram-bot-api"
	"net/http"
	"os"
)

var (
	TOKEN        = ""
	PORT         = ""
	GO_ENV       = ""
	AnnounceList = [][]string{
		{"udp://tracker.openbittorrent.com:80"},
		{"udp://tracker.publicbt.com:80"},
	}
)

func main() {
	TOKEN = os.Getenv("TOKEN")
	if TOKEN == "" {
		Error.Fatalln("$TOKEN not set")
	}

	PORT = os.Getenv("PORT")
	if PORT == "" {
		Error.Fatalln("$PORT not set")
	}

	GO_ENV = os.Getenv("GO_ENV")
	if GO_ENV == "" {
		Warn.Println("$GO_ENV not set")

		//Set default $GO_ENV value to "development"
		GO_ENV = "development"
	}

	Info.Println("Starting bot...")

	bot, err := tbot.NewBotAPI(TOKEN)
	if err != nil {
		Error.Fatalln("Error in starting bot", err.Error())
	}

	if GO_ENV == "development" {
		bot.Debug = false
	}

	Info.Printf("Authorized on account %s\n", bot.Self.UserName)

	updates := fetchUpdates(bot)

	for update := range updates {
		if update.Message == nil {
			//msg := tbot.NewMessage(update.Message.Chat.ID, "Sorry, I am not sure what you mean, Type /help to get help")
			//bot.Send(msg)
			continue
		}

		handleUpdates(bot, update)
	}
}

func fetchUpdates(bot *tbot.BotAPI) tbot.UpdatesChannel {
	if GO_ENV == "development" {
		//Use polling, because testing on local machine

		//Remove webhook
		bot.RemoveWebhook()

		Info.Println("Using Polling Method to fetch updates")
		u := tbot.NewUpdate(0)
		u.Timeout = 60
		updates, err := bot.GetUpdatesChan(u)
		if err != nil {
			Warn.Println("Problem in fetching updates", err.Error())
		}

		return updates

	} else {

		//Remove any existing webhook
		bot.RemoveWebhook()

		//	Use Webhook
		Info.Println("Setting webhooks to fetch updates")
		_, err := bot.SetWebhook(tbot.NewWebhook("https://d2t-bot.herokuapp.com/d2t_converter/" + bot.Token))
		if err != nil {
			Error.Fatalln("Problem in setting webhook", err.Error())
		}

		updates := bot.ListenForWebhook("/d2t_converter/" + bot.Token)

		//redirect users visiting "/" to bot's telegram page
		http.HandleFunc("/", redirectToTelegram)

		http.HandleFunc("/torrent/", serveTorrent)

		Info.Println("Starting HTTPS Server")
		go http.ListenAndServe(":"+PORT, nil)

		w, err := bot.GetWebhookInfo()
		if err != nil {
			Error.Fatalln("Error in fetching webhook info", err.Error())
		}

		Info.Println("URL:", w.URL)
		Info.Println("Is Set?:", w.IsSet())
		return updates
	}
}

func redirectToTelegram(resp http.ResponseWriter, req *http.Request) {
	http.Redirect(resp, req, "https://t.me/d2t-bot", http.StatusTemporaryRedirect)
}

func handleUpdates(bot *tbot.BotAPI, u tbot.Update) {

	if u.Message.IsCommand() {
		switch u.Message.Text {
		case "/start", "/help":
			msg := tbot.NewMessage(u.Message.Chat.ID, "This bot Converts Direct links to Torrent, Provide a valid http link to get started")
			msg.ReplyToMessageID = u.Message.MessageID
			bot.Send(msg)

		default:
			msg := tbot.NewMessage(u.Message.Chat.ID, "Invalid Command")
			msg.ReplyToMessageID = u.Message.MessageID
			bot.Send(msg)
		}
		return
	}

	if u.Message.Text != "" {
		i := &Input{}

		err := i.parseURL(u.Message.Text)
		if err != nil {
			msg := tbot.NewMessage(u.Message.Chat.ID, "Invalid URL")
			bot.Send(msg)
			return
		}

		if i.du.Scheme != "http" {
			msg := tbot.NewMessage(u.Message.Chat.ID, "Only http url scheme is supported")
			bot.Send(msg)
			Warn.Println("Invalid URL Scheme", i.du.String())
			return
		}

		err = i.download()
		if err != nil {
			Warn.Println("Problem in downloading file", err.Error())
			Warn.Println("URL:", i.du.String())

			msg := tbot.NewMessage(u.Message.Chat.ID, "Problem in downloading file, Please retry")
			bot.Send(msg)

			//Delete Downloaded file, because this is probably just a part of something and no reason to keep it
			i.Clean()

			return
		}

		err = i.createTorrent()
		if err != nil {
			Warn.Println("Error in conversion", err.Error())

			msg := tbot.NewMessage(u.Message.Chat.ID, "Error in conversion")
			msg.ReplyToMessageID = u.Message.MessageID
			bot.Send(msg)

			//Delete Downloaded file, because some error occurred in created a torrent
			i.Clean()
			return
		}

		i.save()

		i.Clean()
	}
}
