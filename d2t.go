package main

import (
	tbot "github.com/go-telegram-bot-api/telegram-bot-api"
	"os"
	"net/http"
	"net/url"
	"strings"
	"io"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"time"
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

type Input struct {
	du      *url.URL
	torrent metainfo.MetaInfo
	file    string
}

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
	}
	//Set default $GO_ENV value to "development"
	GO_ENV = "development"

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

		//	Use Webhook, because deploying on heroku
		Info.Println("Setting webhooks to fetch updates")
		_, err := bot.SetWebhook(tbot.NewWebhook("https://dry-hamlet-60060.herokuapp.com/d2t_converter/" + bot.Token))
		if err != nil {
			Error.Fatalln("Problem in setting webhook", err.Error())
		}

		updates := bot.ListenForWebhook("/d2t_converter/" + bot.Token)

		//redirect users visiting "/" to bot's telegram page
		http.HandleFunc("/", redirectToTelegram)

		Info.Println("Starting HTTPS Server")
		go http.ListenAndServeTLS(":"+PORT, "cert.pem", "private.key", nil)

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
	http.Redirect(resp, req, "https://t.me/d2t_converter", http.StatusTemporaryRedirect)
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

			return
		}

		err = i.createTorrent()
		if err != nil {
			Warn.Println("Error in conversion", err.Error())

			msg := tbot.NewMessage(u.Message.Chat.ID, "Error in conversion")
			msg.ReplyToMessageID = u.Message.MessageID
			bot.Send(msg)
			return
		}

		i.save()
	}
}

func (t *Input) parseURL(u string) (error) {
	a, err := url.ParseRequestURI(u)

	if err != nil {
		return err
	}
	t.du = a
	return nil
}

func (t *Input) createTorrent() error {

	mi := metainfo.MetaInfo{
		AnnounceList: AnnounceList,
		CreatedBy:    "d2t_bot(https://t.me/d2t_bot) and https://github.com/anacrolix/torrent",
		CreationDate: time.Now().Unix(),
		URLList: []string{
			t.du.String(),
		},
	}

	info := metainfo.Info{
		PieceLength: 256 * 1024,
	}
	err := info.BuildFromFilePath(t.file)
	if err != nil {
		return err
	}

	mi.InfoBytes, err = bencode.Marshal(info)

	if err != nil {
		return err
	}

	t.torrent = mi
	return nil
}

func (t *Input) save() {
	x, _ := os.Create(t.file + ".torrent")

	t.torrent.Write(x)
	defer x.Close()
}

func (t *Input) download() error {

	resp, err := http.Get(t.du.String())
	if err != nil {
		return err
	}

	p := t.du.Path[strings.LastIndex(t.du.Path, "/")+1:]

	t.file = p

	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()
	io.Copy(f, resp.Body)

	return nil
}
