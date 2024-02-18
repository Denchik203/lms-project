package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"text/template"
	"time"

	"github.com/moxar/arithmetic"
)

const time0 = "2006-01-02 15:04:05"

var (
	HPtpl, _ = template.ParseFiles("tmpl/index.html")
	ECtpl, _ = template.ParseFiles("tmpl/EditConfigPage.html")
	EPtpl, _ = template.ParseFiles("tmpl/ExprPage.html")
	config   = make(map[string]int)
	pool     = New()
	Requests = make([]*Request, 0)
	NextId   = 0
	mux      sync.Mutex
)

type Request struct {
	Id     int
	Status string
	Result string
	Expr   string
	Start  string
	End    string
}

func NewRequest(expr string) *Request {
	NextId++
	return &Request{Id: NextId, Status: "Waiting...", Start: time.Now().Format("2006-01-02 15:04:05"), End: "-", Expr: expr, Result: "-"}
}

type Pool struct {
	Requests     chan *Request
	Done         chan interface{}
	numOfWorkers int
}

func New() *Pool {
	return &Pool{Requests: make(chan *Request), Done: make(chan interface{})}
}

func (p *Pool) Run(r *Request) {
	go func() {
		p.Requests <- r
	}()
}

func (p *Pool) Update() {
	mux.Lock()
	NumOfWorkers := config["NumOfWorkers"]
	mux.Unlock()
	for p.numOfWorkers < NumOfWorkers {
		p.numOfWorkers++
		go func() {
			for {
				select {
				case r := <-p.Requests:
					Process(r)
				case <-p.Done:
					return
				}
			}
		}()
	}
	for p.numOfWorkers > NumOfWorkers {
		select {
		case p.Done <- 1:
			p.numOfWorkers--
		default:
			time.Sleep(time.Millisecond * time.Duration(10))
		}
	}
}

func Process(r *Request) {
	defer func() {
		r.End = time.Now().Format(time0)
	}()
	result, err := arithmetic.Parse(r.Expr)
	if err != nil {
		r.Status = http.StatusText(http.StatusBadRequest)
		return
	}
	r.Status = http.StatusText(http.StatusProcessing)
	timeOfWorking := time.Duration(0)
	for _, i := range r.Expr {
		if _, ok := config[string(i)]; ok {
			timeOfWorking += time.Duration(config[string(i)]) * time.Millisecond
		}
	}
	r.End = time.Now().Add(timeOfWorking).Format(time0)
	log.Println("Expression solving...")
	<-time.After(timeOfWorking)
	log.Println("Expression solved")
	r.Result = fmt.Sprint(result.(float64))
	r.Status = http.StatusText(http.StatusOK)
}

func HomePage(w http.ResponseWriter, r *http.Request) {
	HPtpl.Execute(w, nil)
	r.ParseForm()
	form := r.Form["Expression"]
	if len(form) > 0 && len(form[0]) > 0 {
		if form[0] == "stop" {
			os.Exit(0)
		}
		r := NewRequest(form[0])
		Requests = append(Requests, r)
		pool.Run(r)
	}
}

func StopServer(w http.ResponseWriter, r *http.Request) {
	os.Exit(0)
}

func ExpressionsPage(w http.ResponseWriter, r *http.Request) {
	EPtpl.Execute(w, Requests)
}

func EditConfig(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	var (
		keys   = make([]string, 0)
		values = make([]string, 0)
	)
	for key, val := range r.Form {
		newVal, err := strconv.Atoi(val[0])
		if err == nil && newVal >= 0 {
			mux.Lock()
			config[key] = newVal
			values = append(values, fmt.Sprint(config[key]))
			mux.Unlock()
			keys = append(keys, key)
		}
	}
	ECtpl.Execute(w, config)
	pool.Update()
	f, _ := os.Open("config.csv")
	writer := csv.NewWriter(f)
	writer.Write(keys)
	writer.Write(values)
}

func main() {
	var (
		f, _         = os.OpenFile("config.csv", os.O_RDONLY, 0644)
		Reader       = csv.NewReader(f)
		keys, values []string
	)
	keys, _ = Reader.Read()
	values, _ = Reader.Read()
	mux.Lock()
	for i := range keys {
		config[keys[i]], _ = strconv.Atoi(values[i])
	}
	mux.Unlock()
	pool.Update()
	mux := http.NewServeMux()
	mux.HandleFunc("/", HomePage)
	mux.HandleFunc("/stop", StopServer)
	mux.HandleFunc("/exprs", ExpressionsPage)
	mux.HandleFunc("/editconfig", EditConfig)
	http.ListenAndServe(":8080", mux)
}
