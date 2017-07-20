package main

import (
	"github.com/beeker1121/goque"
	tbot "github.com/go-telegram-bot-api/telegram-bot-api"
	"gopkg.in/mgo.v2"
	"net/http"
	"os"
	"strconv"
	"strings"
	"gopkg.in/mgo.v2/bson"
	"sync"
)

var (
	TOKEN  = ""
	PORT   = ""
	GO_ENV = ""
	dbSess *mgo.Session
	q      *Queue
)

const (
	HOST = "https://d2t-bot.ishanjain.me"
)

type Queue struct {
	*goque.Queue
	cond sync.Cond
	mu   sync.Mutex
}

/*
 * Flow of this bot
 * Take a URL from user
 * Validate URL
 * Send a head request at that url and get detail like length, filename and queue it.
 * Send a "Queued" message with some stats to user
 * Start Processing top item from queue and send a processing message to the user who queued that link
 * Process link and upon completion store it in database and send the a download link to the user
 */

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

	Info.Println("Connecting to Database")

	MONGOADDR := os.Getenv("MONGODB_URI")
	if MONGOADDR == "" {
		MONGOADDR = "mongodb://localhost:27017/burnbitbot"
	}
	dbSess, err = mgo.Dial(MONGOADDR)
	if err != nil {
		Error.Fatalln("Error in connecting to database", err.Error())
	}

	Info.Printf("Authorized on account %s(@%s)\n", bot.Self.FirstName, bot.Self.UserName)

	//Initialise Queue
	q = &Queue{}
	q.cond.L = &q.mu

	//Create a persistent Queue
	q.Queue, err = goque.OpenQueue("download_queue")
	if err != nil {
		Error.Fatalln("Error in creating Download Queue", err.Error())
	}

	Info.Println("Starting Queue Processor")

	updates := fetchUpdates(bot)

	go func() {
		for update := range updates {
			if update.Message == nil {
				//msg := tbot.NewMessage(update.Message.Chat.ID, "Sorry, I am not sure what you mean, Type /help to get help")
				//bot.Send(msg)
				continue
			}

			handleUpdates(bot, update)
		}
	}()

	for {
		Info.Println("Hold Lock")
		q.cond.L.Lock()

		item, err := q.Dequeue()
		if err != nil && err != goque.ErrEmpty {
			Error.Println("Error in Dequeueing", err.Error())
		}

		if err == goque.ErrEmpty {
			Info.Println("Waiting")
			q.cond.Wait()
		}
		Info.Println("Release Lock")
		q.cond.L.Unlock()

		if item != nil {
			//TODO:Only spawn a limited number of routines
			go processQueue(item, bot)
		}
	}
}

func fetchUpdates(bot *tbot.BotAPI) tbot.UpdatesChannel {
	if GO_ENV == "development" {
		//Use polling, because testing on local machine
		//I'll remove this once I complete this bot
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
		_, err := bot.SetWebhook(tbot.NewWebhook(HOST + "/burnbitbot/" + bot.Token))
		if err != nil {
			Error.Fatalln("Problem in setting webhook", err.Error())
		}

		updates := bot.ListenForWebhook("/burnbitbot/" + bot.Token)

		//redirect users visiting "/" to bot's telegram page
		http.HandleFunc("/", redirectToTelegram)

		//The handler for Metainfo Download links
		http.HandleFunc("/torrent/", serveTorrent)

		Info.Println("Starting HTTP Server")
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

func handleUpdates(bot *tbot.BotAPI, u tbot.Update) {

	if u.Message.IsCommand() {
		switch u.Message.Text {
		case "/start", "/help":
			msg := tbot.NewMessage(u.Message.Chat.ID, "This bot Converts Direct links to a Torrent, Provide a valid http link to get started")
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
		i := &DownloadJob{}

		//Validate URL
		err := i.parseURL(u.Message.Text)
		if err != nil {
			msg := tbot.NewMessage(u.Message.Chat.ID, "Invalid URL")
			msg.ReplyToMessageID = u.Message.MessageID
			bot.Send(msg)
			return
		}

		//Make sure it's http/ftp
		if i.DU.Scheme != "http" && i.DU.Scheme != "ftp" {
			msg := tbot.NewMessage(u.Message.Chat.ID, "Only http/ftp url scheme is supported")
			msg.ReplyToMessageID = u.Message.MessageID
			bot.Send(msg)
			Warn.Println("Invalid URL Scheme", i.DU.String())
			return
		}

		//Fetch Metadata about the Filename
		err = i.fetchMetadata()
		if err != nil {
			Warn.Println("Error in fetching metadata", err.Error())
			msg := tbot.NewMessage(u.Message.Chat.ID, "Error in fetching Metadata "+err.Error())
			msg.ReplyToMessageID = u.Message.MessageID
			bot.Send(msg)
			return
		}

		//Store Data about the user
		i.User = UserData{
			MessageID: u.Message.MessageID,
			Username:  u.Message.From.UserName,
			ChatID:    u.Message.Chat.ID,
			UserID:    u.Message.From.ID,
		}

		if item, err := find(i); err == nil {
			Info.Println("Already in Database")
			msg := tbot.NewMessage(u.Message.Chat.ID, "Successful!"+
				"\nLink: "+ HOST+ "/torrent/"+ item.Hash+ ".torrent")

			msg.ReplyToMessageID = u.Message.MessageID
			bot.Send(msg)
			return
		}

		item, err := q.EnqueueObject(i)
		if err != nil {
			Error.Println("Error in Enqueuing item", err.Error())
		}
		q.cond.Broadcast()

		var j DownloadJob

		item.ToObject(&j)

		//Info.Println(item.ID, i.User.Username, j.Filename, j.ContentType, j.Size, j.DU.String())
		msg := tbot.NewMessage(u.Message.Chat.ID,
			"Queued Task \nID, " + strconv.FormatUint(item.ID, 10)+
				"\nName: "+ i.Filename+
				"\nLength: "+ strconv.FormatFloat(i.SizeInMiB, 'f', 4, 64)+ "MiB"+
				"\nType: "+ i.ContentType+
				"\nURL: "+ i.DU.String()+
				"\n\nYou'll notified about the progress")
		bot.Send(msg)

	}
}

func serveTorrent(resp http.ResponseWriter, req *http.Request) {

	hash := strings.Split(req.URL.String(), "/torrent/")[1]
	hash = strings.Split(hash, ".torrent")[0]

	sess := dbSess.Copy()
	c := sess.DB("burnbitbot").C("data")

	di := &DatabaseItem{}
	err := c.Find(bson.M{"hash": hash}).One(di)

	if err != nil {
		if err != mgo.ErrNotFound {
			Error.Println("Error in serving", err.Error())
			http.Error(resp, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
		http.Error(resp, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		Warn.Println("Error in serving", err.Error())
		return
	}

	//Set proper header
	resp.Header().Set("Content-Type", "application/x-bittorrent")
	resp.Write(di.File.Data)
}

func redirectToTelegram(resp http.ResponseWriter, req *http.Request) {
	http.Redirect(resp, req, "https://t.me/burnbitbot", http.StatusTemporaryRedirect)
}
