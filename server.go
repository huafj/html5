package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/julienschmidt/httprouter"
)

type apiObj struct {
	Days    int    `json:"days"`
	AccDays int    `json:"accDays,omitempty"`
	Current int    `json:"current,omitempty"`
	Add     int    `json:"add,omitempty"`
	Title   string `json:"title"`
	ID      int    `json:"id,omitempty"`
}

type Server struct {
	ID       int      `json:"id"`
	Objs     []apiObj `json:"objs"`
	htmlTmpl *template.Template
}

func (srv *Server) createObj(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	body, _ := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))
	obj := apiObj{}
	json.Unmarshal(body, &obj)
	obj.ID = srv.ID
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
	fmt.Printf("update %s\n", string(body))
	json.Unmarshal(body, &obj)
	for i := range srv.Objs {
		if srv.Objs[i].ID == obj.ID {
			srv.Objs[i].Days = obj.Days + obj.Add
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
	fmt.Printf("delete %s\n", string(body))
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
	router.GET("/", srv.getObjs)
	log.Fatal(http.ListenAndServe(":8080", router))
}
