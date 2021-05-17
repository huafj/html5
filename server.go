package main

import (
	"bufio"
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
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"

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
	user        *user.User
	env         []string
}

func (srv *Server) createObj(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	body, _ := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))
	obj := apiObj{}
	json.Unmarshal(body, &obj)
	obj.canUpdate = true
	obj.Current = 0
	obj.AccDays = 0
	obj.ID = srv.ID + 1
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
			title := srv.Objs[i].Title
			srv.Objs[i].Title = obj.Title
			switch {
			case obj.Add == 0:
				if title != obj.Title {
					srv.Objs[i].Current = 0
					srv.Objs[i].AccDays = 0
					body, _ = json.MarshalIndent(srv.Objs[i], "", " ")
				}
			case srv.Objs[i].canUpdate || srv.forceUpdate:
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
				srv.Objs[i].Days = obj.Days
				srv.Objs[i].Gift = obj.Gift
			}
			fmt.Fprintf(w, string(body))
			break
		}
	}
	go srv.Save()
}

func (srv *Server) deleteObj(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	body, _ := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))
	obj := apiObj{}
	json.Unmarshal(body, &obj)
	for i := range srv.Objs {
		if srv.Objs[i].ID == obj.ID &&
			srv.Objs[i].Current >= srv.Objs[i].Days {
			obj = srv.Objs[i]
			if i == len(srv.Objs)-1 {
				srv.Objs = srv.Objs[:i]
			} else {
				srv.Objs = append(srv.Objs[:i], srv.Objs[i+1:]...)
			}
			go srv.Save()
			break
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
				cmd := exec.Command("ffmpeg", "-y", "-i", filepath.Join("audio", fileName),
					"-vn", "-f", "mp3", filepath.Join("audio", id+".mp3"))
				if srv.debug {
					fmt.Printf("%v\n", cmd)
				}
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

func (srv *Server) exec(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	command := params.ByName("cmd")
	cmd := exec.Command(command)
	if srv.user != nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
		uid, _ := strconv.Atoi(srv.user.Uid)
		gid, _ := strconv.Atoi(srv.user.Gid)
		cmd.Dir = srv.user.HomeDir
		cmd.Env = srv.env
		cmd.SysProcAttr.Credential = &syscall.Credential{
			Uid: uint32(uid),
			Gid: uint32(gid),
		}
		if groups, err := srv.user.GroupIds(); err == nil {
			for _, group := range groups {
				gid, _ := strconv.Atoi(group)
				cmd.SysProcAttr.Credential.Groups = append(cmd.SysProcAttr.Credential.Groups, uint32(gid))
			}
		}
	}

	conn, rwbuf, _ := w.(http.Hijacker).Hijack()
	defer conn.Close()
	ptmx, err := pty.Start(cmd)
	pty.InheritSize(os.Stdin, ptmx)
	if err == nil {
		go io.Copy(ptmx, rwbuf)
		go io.Copy(rwbuf, ptmx)
		cmd.Wait()
		ptmx.Close()
	}
	return
}

func (srv *Server) Save() {
	body, err := json.MarshalIndent(srv, "", " ")
	if err == nil {
		if w, err := os.OpenFile(srv.config, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(0744)); err == nil {
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
	userName := flag.String("user", "root", "")
	workDir := filepath.Dir(os.Args[0])
	flag.Parse()
	os.Chdir(workDir)
	htmlBody, err := ioutil.ReadFile("index.html")
	if err != nil {
		panic(err)
	}
	if len(*userName) > 0 {
		srv.user, _ = user.Lookup(*userName)
		if srv.user != nil {
			env := make([]string, 0, 0)
			if output, err := exec.Command("sudo", "-u", *userName, "env").Output(); err == nil {
				scanner := bufio.NewScanner(strings.NewReader(string(output)))
				for scanner.Scan() {
					line := scanner.Text()
					env = append(env, line)
				}
				srv.env = env
			} else {
				panic(err)
			}
		}
	}

	srv.htmlTmpl = template.Must(template.New("html").Parse(string(htmlBody)))

	fwBody, err := ioutil.ReadFile("firework.html")
	if err != nil {
		panic(err)
	}

	srv.fwTmpl = template.Must(template.New("firework").Parse(string(fwBody)))
	body, _ := ioutil.ReadFile(srv.config)
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

	go func() {
		for {
			<-time.Tick(time.Hour * 12)
			now := time.Now()
			days := now.YearDay() - srv.Updated.Year()
			if days < 0 {
				days = 0
			}
			for i := range srv.Objs {
				srv.Objs[i].canUpdate = true
				srv.Objs[i].AccDays += days
			}
			if days > 0 {
				srv.Updated = now
				srv.Save()
			}
		}
	}()

	for i := range srv.Objs {
		srv.Objs[i].canUpdate = canUpdate
		if canUpdate {
			srv.Objs[i].AccDays += 1
		}
	}
	srv.workDir, _ = os.Getwd()
	if fi, err := os.Stat(filepath.Join("img", srv.Background)); err != nil || !fi.Mode().IsRegular() || len(srv.Background) == 0 {
		srv.Background = "frozen.jpg"
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
	router.GET("/exec/:cmd", srv.exec)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), router))
}
