// pic_cache project main.go
package main

import (
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

var APP_DIR, CACHE_DIR string
var makingId map[string]chan bool

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	exePath, _ := exec.LookPath(os.Args[0])
	APP_DIR, _ := filepath.Split(exePath)
	CACHE_DIR = filepath.Join(APP_DIR, "cache")

	if err := os.MkdirAll(CACHE_DIR, 0777); err != nil {
		log.Fatalln("Create cache dir error!", err)
	} else {
		log.Println("Cache dir:", CACHE_DIR)
	}

	makingId = make(map[string]chan bool)

	http.HandleFunc("/pic", getPic)
	http.HandleFunc("/job", getPicJob)
	http.ListenAndServe(":2537", nil)
}

//图片代理
func getPic(w http.ResponseWriter, req *http.Request) {
	var cacheContent *[]byte
	var err error

	req.ParseForm()
	picUrl := req.Form.Get("url")
	if picUrl == "" {
		w.Write([]byte("param error"))
		return
	}
	picId := getCacheId(picUrl)
	log.Println("requesting pic id:", picId)

	if lock, ok := makingId[picId]; ok { //证明正在做这个id的cache
		log.Println("another one doing cache for pic id:", picId)
		<-lock //完成了
		close(lock)
		delete(makingId, picId)
	}

	if !cacheExist(picId) {
		log.Println("cache miss, id:", picId)
		cacheContent, err = makeCache(picUrl)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	} else {
		log.Println("cache hit, pic id:", picId)
		//读取缓存
		cacheContent, err = cacheRead(picId)
		if err != nil {
			log.Println("cache read error, id:", picId, "error:", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	w.Write(*cacheContent)
	log.Println("serve pic done, id:", picId)
}

//任务预处理
func getPicJob(w http.ResponseWriter, req *http.Request) {
	req.ParseForm()
	for _, url := range req.PostForm["url"] {
		go makeCache(url)
	}
	w.Write([]byte("Job Received!"))
}

//制作缓存
func makeCache(url string) (data *[]byte, err error) {
	picId := getCacheId(url)
	makingId[picId] = make(chan bool)
	defer func() {
		makingId[picId] <- false
	}()

	log.Println("making cache, url:", url)

	//请求远端
	resp, err := http.Get(url)
	if err != nil {
		log.Println("request remote fail, url:", url)
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := ioutil.ReadAll(resp.Body)
	log.Println("request remote done, size:", len(respBody), "id:", picId)

	//存入缓存
	err = cacheWrite(picId, &respBody)
	if err != nil {
		log.Println("cache save fail, id:", picId)
		return nil, err
	} else {
		log.Println("cache save secceed, id:", picId)
		return &respBody, nil
	}
}
