package main

import (
	"github.com/beeker1121/goque"
	tbot "github.com/go-telegram-bot-api/telegram-bot-api"
	"strconv"
)

func processQueue(item *goque.Item, bot *tbot.BotAPI, done chan bool) {
	defer func() { done <- true }()
	var dj DownloadJob

	err := item.ToObject(&dj)
	if err != nil {
		Error.Println("Error in converting item to object", err.Error())
		return
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
		return
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
		return
	}

	err = dj.save()
	if err != nil {
		msg := tbot.NewMessage(dj.User.ChatID, "Failed in Saving Hash "+dj.DU.String())
		bot.Send(msg)

		Warn.Println("Failed to save Hash", err.Error())
		return
	}

	//Remove the Downloaded file
	err = dj.Clean()
	if err != nil {
		Error.Println("Error in cleaning", err.Error())
	}

	//Everything's Done! Send a Download link to User
	msg = tbot.NewMessage(dj.User.ChatID, "Successful!"+
		"\nLink: "+ HOST+ "/torrent/"+ dj.Metainfo.HashInfoBytes().String()+ ".torrent")
	msg.ReplyToMessageID = dj.User.MessageID
	bot.Send(msg)

}
