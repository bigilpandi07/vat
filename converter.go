package main

import (
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Input struct {
	du      *url.URL
	torrent metainfo.MetaInfo
	file    string
}

func (t *Input) parseURL(u string) error {
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
	x, _ := os.Create(t.file + "." + t.torrent.HashInfoBytes().String() + ".torrent")

	t.torrent.Write(x)
	defer x.Close()
}

func (t *Input) Clean() {
	err := os.RemoveAll(t.file)
	if err != nil {
		Error.Println("Error in deleting file/Folder", err.Error())
	}
}

func (t *Input) download() error {

	resp, err := http.Get(t.du.String())
	if err != nil {
		return err
	}

	//Get the file name
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
