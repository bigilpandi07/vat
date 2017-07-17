package main

import (
	"github.com/fatih/color"
	"log"
	"os"
)

var (
	Info  *log.Logger
	Warn  *log.Logger
	Error *log.Logger
)

func init() {
	Info = log.New(os.Stdout,
		color.GreenString("[INFO] "),
		log.Ltime|log.Lshortfile)

	Warn = log.New(os.Stdout,
		color.YellowString("[WARN] "),
		log.Ltime|log.Lshortfile)

	Error = log.New(os.Stdout,
		color.RedString("[ERROR] "),
		log.Ltime|log.Lshortfile)

}
