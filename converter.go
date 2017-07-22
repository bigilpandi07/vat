package main

import (
	"bytes"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/getlantern/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
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
	User        []UserData
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

type MongoQueueRecord struct {
	URL string `bson:"_id"`
	ID  uint64 `bson:"job_id"`
}

//Parse URL and make sure it's a valid one
func (t *DownloadJob) parseURL(u string) error {
	a, err := url.ParseRequestURI(u)

	if err != nil {
		return err
	}
	t.DU = a
	//Set Filename name
	t.Filename = t.DU.Path[strings.LastIndex(t.DU.Path, "/")+1:]

	return nil
}

//Fetch Metadata about provided link, like it's size and content type
func (t *DownloadJob) fetchMetadata() error {
	resp, err := http.Head(t.DU.String())
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK || resp.ContentLength == 0 {
		return errors.New("Status Code is NOT OK(" + strconv.Itoa(resp.StatusCode) + ")")
	}

	t.Size = resp.ContentLength
	t.ContentType = resp.Header.Get("Content-Type")
	t.SizeInMiB = float64(t.Size) / (1024 * 1024)
	return nil
}

//Save To database
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
		Hash:        t.Metainfo.HashInfoBytes().String(),
		File:        bson.Binary{Data: a.Bytes(), Kind: 0},
	})
	if err != nil {
		return err
	}
	//Info.Println("Saved", t.DU.String())
	return nil
}

//Find if a link like the one provided already exists.
//TODO:Make a smarter find function
//Currently a url like dl.ishanjain.me/xyz and dl.ishanjain.me/?id=abc are different
//Even if they return the exact same file
//A smarter find function'll send a request to a url after removing all query parameters
//And then send a request with all provided query parameters and then decide by
//Checking content length, type and E-tags(if it has etags) to make sure if it's same
//If it is then it'll proceed to search the database after removing all query parameters
//If there is a result, return that, If there is no such url, go through the normal process
func findindb(t *DownloadJob) (*DatabaseItem, error) {
	sess := dbSess.Copy()
	c := sess.DB("burnbitbot").C("data")

	di := &DatabaseItem{}
	err := c.Find(bson.M{"_id": t.DU.String(), "size": t.Size, "filename": t.Filename}).One(di)

	if err != nil {
		return nil, err
	}

	return di, nil
}

func findinQueue(t *DownloadJob) (*MongoQueueRecord, error) {
	sess := dbSess.Copy()
	c := sess.DB("burnbitbot").C("queue")

	u := &MongoQueueRecord{}
	err := c.Find(bson.M{"_id": t.DU.String()}).One(&u)
	if err != nil {
		if err != mgo.ErrNotFound {
			return nil, err
		}

		if err == mgo.ErrNotFound {
			return nil, nil
		}
	}
	return u, nil
}

func storeURLInMongoDB(t *DownloadJob, id uint64) error {
	sess := dbSess.Copy()
	c := sess.DB("burnbitbot").C("queue")

	err := c.Insert(bson.M{"_id": t.DU.String(), "job_id": id})

	if err != nil {
		return err
	}
	return nil
}

//Removes Downloaded Files
func (t *DownloadJob) Clean() error {
	err := os.RemoveAll(t.Filename)
	if err != nil {
		return err
	}
	return nil
}

//Download the file
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

//Create a Meta file
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
