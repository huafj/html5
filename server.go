package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/julienschmidt/httprouter"
)

type apiObj struct {
	Days    int    `json:"days"`
	AccDays int    `json:"accDays"`
	Current int    `json:"current"`
	Add     int    `json:"add,omitempty"`
	Title   string `json:"title"`
	ID      int    `json:"id"`
}

func updateObj(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	body, _ := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))
	obj := apiObj{}
	json.Unmarshal(body, &obj)
	obj.Current += 1
	body, _ = json.Marshal(obj)
	fmt.Fprintf(w, string(body))
}

func createObj(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	todoid := params.ByName("todoid")
	body, _ := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))
	fmt.Fprintf(w, "modifyTodo %s to %s\n", todoid, body)
}

func deleteObj(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	todoid := params.ByName("todoid")
	body, _ := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))
	fmt.Fprintf(w, "modifyTodo %s to %s\n", todoid, body)
}

func getObjs(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	body, _ := os.Open("index.html")
	io.Copy(w, body)
	body.Close()

}

func main() {
	router := httprouter.New()
	router.POST("/api/update", updateObj)
	router.POST("/api/create", createObj)
	router.DELETE("/api/delete", deleteObj)
	router.GET("/", getObjs)
	log.Fatal(http.ListenAndServe(":8080", router))
}
