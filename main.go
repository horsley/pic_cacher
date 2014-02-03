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
	"time"
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
		go func() {
			for { //等待makeCache完成
				if lock, ok := makingId[picId]; ok {
					<-lock //完成了
					close(lock)
					delete(makingId, picId)
					break
				}
				runtime.Gosched()
			}
		}()
		cacheContent, err = makeCache(picUrl)
		if err != nil {
			log.Println("make cache error, id:", picId, "error:", err)
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
		go func() {
			picId := getCacheId(url)
			if !cacheExist(picId) {
				makeCache(url)
			}
		}()
	}
	w.Write([]byte("Job Received!"))
}

//制作缓存
func makeCache(url string) (data *[]byte, err error) {
	var respBody []byte
	picId := getCacheId(url)
	makingId[picId] = make(chan bool)
	defer func() {
		makingId[picId] <- false
		log.Println("makeCache done, id:", picId)
	}()

	log.Println("making cache, url:", url)

	//请求远端
	for i := 0; i < 5; i++ { //最大重试次数
		resp, err := http.Get(url)
		if err != nil {
			log.Println("request remote fail, exiting, url:", url)
			return nil, err
		}
		if resp.StatusCode != 200 {
			log.Println("request remote fail, code:", resp.StatusCode, "waiting for retry")
			time.Sleep((i + 1) * 500 * time.Millisecond)
		} else {
			respBody, _ = ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			log.Println("request remote done, size:", len(respBody), "id:", picId)
		}

	}

	//存入缓存
	err = cacheWrite(picId, &respBody)
	if err != nil {
		log.Println("cache save fail, id:", picId)
		return nil, err
	} else {
		log.Println("cache save succeed, id:", picId)
		return &respBody, nil
	}
}
