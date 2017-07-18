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
	"github.com/getlantern/errors"
	"strconv"
)

type DownloadJob struct {
	DU          *url.URL
	Metainfo    metainfo.MetaInfo
	File        string
	ContentType string
	Size        int64
	User        UserData
}

type UserData struct {
	UserID    int
	Username  string
	MessageID int
	ChatID    int64
}

func (t *DownloadJob) parseURL(u string) error {
	a, err := url.ParseRequestURI(u)

	if err != nil {
		return err
	}
	t.DU = a
	//Set File name
	p := t.DU.Path[strings.LastIndex(t.DU.Path, "/")+1:]
	t.File = p

	return nil
}

func (t *DownloadJob) fetchMetadata() error {
	resp, err := http.Head(t.DU.String())
	if err != nil {
		return err

	}

	if resp.StatusCode != http.StatusOK {
		return errors.New("Status Code is NOT OK(" + strconv.Itoa(resp.StatusCode) + ")")
	}

	t.Size = resp.ContentLength
	t.ContentType = resp.Header.Get("Content-Type")

	return nil
}

func (t *DownloadJob) save() {
	x, _ := os.Create(t.File + "." + t.Metainfo.HashInfoBytes().String() + ".Metainfo")

	t.Metainfo.Write(x)
	defer x.Close()
}

func (t *DownloadJob) Clean() {
	err := os.RemoveAll(t.File)
	if err != nil {
		Error.Println("Error in deleting File/Folder", err.Error())
	}
}

func (t *DownloadJob) download() error {

	resp, err := http.Get(t.DU.String())
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return errors.New("Error in downloading file", err.Error())
	}

	f, err := os.Create(t.File)
	if err != nil {
		return err
	}
	defer f.Close()

	io.Copy(f, resp.Body)

	return nil
}

func (t *DownloadJob) convert() error {

	mi := metainfo.MetaInfo{
		AnnounceList: AnnounceList,
		CreatedBy:    "d2t_bot(https://t.me/d2t_bot) and https://github.com/anacrolix/Metainfo",
		CreationDate: time.Now().Unix(),
		UrlList: []string{
			t.DU.String(),
		},
	}

	info := metainfo.Info{
		PieceLength: 256 * 1024,
	}
	err := info.BuildFromFilePath(t.File)
	if err != nil {
		return err
	}

	mi.InfoBytes, err = bencode.Marshal(info)

	if err != nil {
		return err
	}

	t.Metainfo = mi
	return nil
}
