package main

import (
	"net/http"
	"strconv"
	tbot"github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/beeker1121/goque"
	"time"
)

func serveTorrent(resp http.ResponseWriter, req *http.Request) {
	resp.Write([]byte(req.URL.String()))
}

func startQueueProcessor(bot *tbot.BotAPI) {
	for {
		if item, err := q.Dequeue(); err == nil {

			var dj DownloadJob

			err := item.ToObject(&dj)
			if err != nil {
				Error.Println("Error in converting item to object", err.Error())
				continue
			}
			Info.Println("Processing", dj.DU.String())
			Info.Println("Size", dj.SizeInMiB)

			//Send a message to user as well
			msg := tbot.NewMessage(dj.User.ChatID,
				"Downloading "+
					"\nName: "+ dj.Filename+
					"\nLength: "+ strconv.FormatFloat(float64(dj.Size)/(1024*1024), 'f', 4, 64)+ "MiB"+
					"\nType: "+ dj.ContentType+
					"\nURL: "+ dj.DU.String())

			bot.Send(msg)
			//TODO:Process more than one items at a time if there is more space than what we need for the task at hand

			err = dj.download()

			if err != nil {
				msg := tbot.NewMessage(dj.User.ChatID,
					"Failed in Downloading " + dj.DU.String()+
						"\nReason: "+ err.Error())

				bot.Send(msg)
				continue
			}

			msg = tbot.NewMessage(dj.User.ChatID,
				"Calculating Hash "+
					"\nName: "+ dj.Filename+
					"\nLength: "+ strconv.FormatFloat(float64(dj.Size)/(1024*1024), 'f', 4, 64)+ "MiB"+
					"\nType: "+ dj.ContentType+
					"\nURL: "+ dj.DU.String())

			bot.Send(msg)

			err = dj.convert()
			if err != nil {
				msg = tbot.NewMessage(dj.User.ChatID,
					"Error in Calculating Hash "+
						"\nReason: "+ err.Error())
				bot.Send(msg)

				Warn.Println("Error in calculating Hash", err.Error())
				continue
			}

			err = dj.save()
			if err != nil {
				msg := tbot.NewMessage(dj.User.ChatID, "Failed in Saving Hash "+dj.DU.String())
				bot.Send(msg)

				Warn.Println("Failed to save Hash", err.Error())
			}

			//Everything's Done! Send a Download link to User
			i, err := find(dj)
			if err != nil {
				msg = tbot.NewMessage(dj.User.ChatID, "Failed in Saving to Database !")
				bot.Send(msg)
				continue
			}
			msg = tbot.NewMessage(dj.User.ChatID, "Successful!"+
				"Link: "+ HOST+ "/torrent/"+ i.Hash)
			bot.Send(msg)

		} else {

			if err != goque.ErrEmpty {
				Error.Println("Error in Dequeueing", err.Error())
			}
			continue
			time.After(time.Duration(2) * time.Second)
		}
	}
}
