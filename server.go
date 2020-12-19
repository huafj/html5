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
	"time"

	"github.com/julienschmidt/httprouter"
)

type apiObj struct {
	Days      int    `json:"days"`
	AccDays   int    `json:"accDays,omitempty"`
	Current   int    `json:"current,omitempty"`
	Add       int    `json:"add,omitempty"`
	Title     string `json:"title"`
	Gift      string `json:"gift"`
	ID        int    `json:"id,omitempty"`
	canUpdate bool
}

type Server struct {
	ID          int       `json:"id"`
	Updated     time.Time `json:"updated"`
	Objs        []apiObj  `json:"objs"`
	DoneObjs    []apiObj  `json:"done,omitempty"`
	TempObjs    []apiObj  `json:"-"`
	htmlTmpl    *template.Template
	config      string
	forceUpdate bool
}

func (srv *Server) createObj(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	body, _ := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))
	obj := apiObj{}
	json.Unmarshal(body, &obj)
	obj.ID = srv.ID
	obj.canUpdate = true
	srv.ID++
	srv.Objs = append(srv.Objs, obj)
	fmt.Fprintf(w, string(body))
	go func() {
		srv.Save()
	}()
}

func (srv *Server) updateObj(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	body, _ := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))
	obj := apiObj{}
	if err := json.Unmarshal(body, &obj); err != nil {
		fmt.Printf("update error %v\n", err)
	}
	for i := range srv.Objs {
		if srv.Objs[i].ID == obj.ID {
			if obj.Add != 0 && (srv.Objs[i].canUpdate || srv.forceUpdate) {
				srv.Objs[i].canUpdate = false
				srv.Objs[i].Current += obj.Add
				if srv.Objs[i].Current < 0 {
					srv.Objs[i].Current = 0
				}
			}
			srv.Objs[i].Days = obj.Days
			srv.Objs[i].Gift = obj.Gift
			fmt.Fprintf(w, string(body))
			go func() {
				srv.Save()
			}()
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
			go func() {
				srv.Save()
			}()
		}
	}
}

func (srv *Server) getObjs(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	srv.TempObjs = srv.Objs
	srv.htmlTmpl.Execute(w, srv)
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
	flag.StringVar(&srv.config, "config", "config.json", "config file")
	flag.Parse()

	canUpdate := false
	if srv.Updated.IsZero() {
		canUpdate = true
		srv.Updated = time.Now()
		srv.ID = 0
	} else {
		now := time.Now()
		if now.Sub(srv.Updated).Hours() > 23.0 {
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

	htmlBody, err := ioutil.ReadFile("index.html")
	if err != nil {
		panic(err)
	}
	srv.htmlTmpl = template.Must(template.New("html").Parse(string(htmlBody)))
	body, _ := ioutil.ReadFile("config.json")
	json.Unmarshal(body, srv)

	router := httprouter.New()
	router.POST("/api/update", srv.updateObj)
	router.POST("/api/create", srv.createObj)
	router.DELETE("/api/delete", srv.deleteObj)
	router.ServeFiles("/img/*filepath", http.Dir("img"))
	router.GET("/", srv.getObjs)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), router))
}
