package main

import "net/http"

//When Bot is polling for updates there is no HTTP Server,
//So I'll have to create a http server to serve torrents
//When Bot is running using a webhook, I can use the server
//I started for webhook

func serveTorrents() {

	if GO_ENV == "development" {
		//Development and Polling Mode
	} else {
		//	Production, Or Webhook mode
	}
}

func serveTorrent(resp http.ResponseWriter, req *http.Request) {
	resp.Write([]byte(req.URL.String()))
}
