package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"
)

const MaxFileSize = (1 << 20) * 200

type apiObj struct {
	Days      int    `json:"days"`
	AccDays   int    `json:"accDays"`
	Current   int    `json:"current"`
	Add       int    `json:"add,omitempty"`
	Title     string `json:"title"`
	Gift      string `json:"gift"`
	ID        int    `json:"id"`
	canUpdate bool
}

type Server struct {
	ID          int       `json:"id"`
	Updated     time.Time `json:"updated"`
	Objs        []apiObj  `json:"objs"`
	DoneObjs    []apiObj  `json:"done,omitempty"`
	TempObjs    []apiObj  `json:"-"`
	Background  string    `json:"backgroud"`
	htmlTmpl    *template.Template
	fwTmpl      *template.Template
	config      string
	workDir     string
	forceUpdate bool
	debug       bool
}

func (srv *Server) createObj(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	body, _ := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))
	obj := apiObj{}
	json.Unmarshal(body, &obj)
	obj.ID = srv.ID + 1
	obj.canUpdate = true
	obj.Current = 0
	obj.AccDays = 0
	srv.ID++
	srv.Objs = append(srv.Objs, obj)
	fmt.Fprintf(w, string(body))
	go srv.Save()
}

func (srv *Server) updateObj(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	body, _ := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))
	obj := apiObj{}
	if err := json.Unmarshal(body, &obj); err != nil {
		log.Printf("update error %v\n", err)
	}
	for i := range srv.Objs {
		if srv.Objs[i].ID == obj.ID {
			srv.Objs[i].Title = obj.Title
			if obj.Add != 0 && (srv.Objs[i].canUpdate || srv.forceUpdate) {
				if srv.debug {
					log.Printf("udate %s obj=%v forcUpdate=%v\n", string(body), srv.Objs[i], srv.forceUpdate)
				}
				srv.Objs[i].canUpdate = false
				srv.Objs[i].Current += obj.Add
				if srv.Objs[i].Current < 0 {
					srv.Objs[i].Current = 0
				} else if srv.Objs[i].Current >= srv.Objs[i].Days {
					srv.Objs[i].Current = srv.Objs[i].Days
				}
			}
			srv.Objs[i].Days = obj.Days
			srv.Objs[i].Gift = obj.Gift
			fmt.Fprintf(w, string(body))
			go srv.Save()
			break
		}
	}
}

func (srv *Server) deleteObj(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	body, _ := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))
	obj := apiObj{}
	json.Unmarshal(body, &obj)
	for i := range srv.Objs {
		if srv.Objs[i].ID == obj.ID {
			if i == len(srv.Objs)-1 {
				srv.Objs = srv.Objs[:i]
			} else {
				srv.Objs = append(srv.Objs[:i], srv.Objs[i+1:]...)
			}
			go srv.Save()
		}
	}
}

func (srv *Server) getObjs(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	srv.TempObjs = srv.Objs
	srv.htmlTmpl.Execute(w, srv)
}

func (srv *Server) cong(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	type soundPath struct {
		Existed bool
		Path    string
	}
	sp := soundPath{}
	id := r.URL.Query().Get("id")
	path := fmt.Sprintf("audio/%s.mp3", id)

	if _, err := os.Stat(path); err == nil {
		sp.Existed = true
		sp.Path = "/" + path
	}
	log.Printf("id=%s,%s path=%s %v\n", id, r.RequestURI, path, sp.Existed)

	w.Header().Add("Content-Type", "text/html;charset=utf-8")
	srv.fwTmpl.Execute(w, sp)
}

func (srv *Server) uploadFile(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	r.ParseMultipartForm(MaxFileSize)
	file, handler, err := r.FormFile("image_file")
	id := r.FormValue("id")
	if err != nil {
		log.Printf("Error Retrieving the File %v", err)
		return
	}
	defer func() {
		file.Close()
		if r.Body != nil {
			r.Body.Close()
		}
	}()

	if srv.debug {
		fmt.Printf("Uploaded File: %+v\n", handler.Filename)
		fmt.Printf("File Size: %+v\n", handler.Size)
		fmt.Printf("MIME Header: %+v %s\n", handler.Header, id)
	}
	fileName := strings.ToLower(handler.Filename)
	if strings.HasSuffix(fileName, ".jpg") || strings.HasSuffix(fileName, ".jpeg") {
		wf, _ := os.OpenFile(filepath.Join("img", fileName), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(0744))
		if _, err := io.Copy(wf, file); err == nil {
			srv.Background = fileName
			if srv.debug {
				log.Printf("Background=%s\n", srv.Background)
			}
			go srv.Save()
		} else {
			log.Printf("%v", err)
		}
		wf.Close()
		fmt.Fprintf(w, "/audio/%s", fileName)
		return
	}
	if strings.HasSuffix(fileName, ".avi") ||
		strings.HasSuffix(fileName, ".mp4") ||
		strings.HasSuffix(fileName, ".mov") {
		wf, _ := os.OpenFile(filepath.Join("audio", fileName), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(0744))
		if _, err := io.Copy(wf, file); err == nil {
			wf.Close()
			go func() {
				cmd := exec.Command("ffmpeg", "-i", filepath.Join("audio", fileName),
					"-vn", "-f", "mp3", filepath.Join("audio", id+".mp3"))

				fmt.Printf("%v\n", cmd)
				if out, err := cmd.Output(); err != nil {
					log.Printf("%v %s", err, string(out))
				}
				cmd = exec.Command("rm", "-f", filepath.Join("audio", fileName))
				cmd.Run()
			}()
		}
		return
	}
	if strings.HasSuffix(fileName, ".mp3") {
		wf, _ := os.OpenFile(filepath.Join("audio", id+".mp3"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(0744))
		io.Copy(wf, file)
		wf.Close()
	}
}

func (srv *Server) Save() {
	body, err := json.MarshalIndent(srv, "", " ")
	if err == nil {
		if w, err := os.OpenFile("config.json", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(0744)); err == nil {
			fmt.Fprintf(w, string(body))
			w.Close()
		} else {
			fmt.Printf("%v\n", err)
		}
	} else {
		fmt.Printf("%v\n", err)
	}
}

func main() {
	srv := &Server{Objs: make([]apiObj, 0, 0)}
	port := flag.Int("port", 80, "port")
	flag.BoolVar(&srv.forceUpdate, "force", false, "force update")
	flag.BoolVar(&srv.debug, "debug", false, "debug")
	flag.StringVar(&srv.config, "config", "config.json", "config file")
	flag.Parse()

	htmlBody, err := ioutil.ReadFile("index.html")
	if err != nil {
		panic(err)
	}
	srv.htmlTmpl = template.Must(template.New("html").Parse(string(htmlBody)))

	fwBody, err := ioutil.ReadFile("firework.html")
	if err != nil {
		panic(err)
	}

	srv.fwTmpl = template.Must(template.New("firework").Parse(string(fwBody)))
	body, _ := ioutil.ReadFile("config.json")
	json.Unmarshal(body, srv)

	canUpdate := false
	if srv.Updated.IsZero() {
		canUpdate = true
		srv.Updated = time.Now()
		srv.ID = 0
	} else {
		now := time.Now()
		if now.Sub(srv.Updated).Hours() > 12.0 {
			canUpdate = true
			srv.Updated = now
		}
	}

	for i := range srv.Objs {
		srv.Objs[i].canUpdate = canUpdate
		if canUpdate {
			srv.Objs[i].AccDays += 1
		}
	}
	srv.workDir, _ = os.Getwd()
	if len(srv.Background) == 0 {
		srv.Background = "frozen.jpeg"
	}
	router := httprouter.New()
	router.POST("/api/update", srv.updateObj)
	router.POST("/api/create", srv.createObj)
	router.DELETE("/api/delete", srv.deleteObj)
	router.ServeFiles("/img/*filepath", http.Dir("img"))
	router.ServeFiles("/audio/*filepath", http.Dir("audio"))
	router.ServeFiles("/js/*filepath", http.Dir("js"))
	router.GET("/", srv.getObjs)
	router.GET("/firework", srv.cong)
	router.POST("/upload", srv.uploadFile)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), router))
}
