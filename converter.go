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
	"gopkg.in/mgo.v2/bson"
	"bytes"
)

var AnnounceList = [][]string{
	{"udp://tracker.openbittorrent.com:80"},
	{"udp://tracker.publicbt.com:80"},
}

type DownloadJob struct {
	DU          *url.URL
	Metainfo    metainfo.MetaInfo
	Filename    string
	ContentType string
	Size        int64
	SizeInMiB   float64
	User        UserData
}

type UserData struct {
	UserID    int
	Username  string
	MessageID int
	ChatID    int64
}

type DatabaseItem struct {
	Filename    string
	DU          string
	ContentType string
	Size        int64
	Hash        string
	File        bson.Binary
}

func (t *DownloadJob) parseURL(u string) error {
	a, err := url.ParseRequestURI(u)

	if err != nil {
		return err
	}
	t.DU = a
	//Set Filename name
	p := t.DU.Path[strings.LastIndex(t.DU.Path, "/")+1:]
	t.Filename = p

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
	t.SizeInMiB = float64(t.Size) / (1024 * 1024)
	return nil
}

func (t *DownloadJob) save() error {
	sess := dbSess.Copy()

	c := sess.DB("burnbitbot").C("data")

	var a bytes.Buffer

	t.Metainfo.Write(&a)

	_, err := c.Upsert(bson.M{"_id": t.DU.String()}, &DatabaseItem{
		DU:          t.DU.String(),
		Size:        t.Size,
		ContentType: t.ContentType,
		Filename:    t.Filename,
		Hash:        string(t.Metainfo.HashInfoBytes().String()),
		File:        bson.Binary{Data: a.Bytes(), Kind: 0},
	})
	if err != nil {
		return err
	}
	Info.Println("Saved", t.DU.String())
	return nil
}

func find(t *DownloadJob) (*DatabaseItem, error) {
	sess := dbSess.Copy()
	c := sess.DB("burnbitbot").C("data")

	di := &DatabaseItem{}
	err := c.Find(bson.M{"_id": t.DU.String(), "size": t.Size, "filename": t.Filename}).One(di)

	if err != nil {
		return nil, err
	}

	return di, nil
}

func (t *DownloadJob) Clean() error {
	err := os.RemoveAll(t.Filename)
	if err != nil {
		return err
	}
	return nil
}

func (t *DownloadJob) download() error {

	resp, err := http.Get(t.DU.String())
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return errors.New("Error in downloading file", err.Error())
	}

	f, err := os.Create(t.Filename)
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
	err := info.BuildFromFilePath(t.Filename)
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
