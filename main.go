// pic_cache project main.go
package main

import (
	"encoding/base64"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var APP_DIR, CACHE_DIR string
var makingId map[string]*sync.Mutex
var picFailCount map[string]int
var picSuccessCount map[string]int
var makingIdLock, failCountLock, successCountLock *sync.RWMutex

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

	makingId = make(map[string]*sync.Mutex)
	picFailCount = make(map[string]int)
	makingIdLock = &sync.RWMutex{}
	failCountLock = &sync.RWMutex{}
	successCountLock = &sync.RWMutex{}

	http.HandleFunc("/pic", getPic)
	http.HandleFunc("/job", getPicJob)
	http.HandleFunc("/fail2refresh.js", fail2refresh)
	http.ListenAndServe(":2537", nil)
}

//图片代理
func getPic(w http.ResponseWriter, req *http.Request) {
	var cacheContent *[]byte
	var err error

	req.ParseForm()
	picUrlcoded := req.Form.Get("url")
	if picUrlcoded == "" {
		w.Write([]byte("param error"))
		return
	}
	coding := base64.NewEncoding("VPQRXAZabBCDNkYcWMIist5EFLvlmnGHu34wxyz0hSTJKOdefgU6j12opqr978-_")
	picUrldecoded, err := coding.DecodeString(picUrlcoded)
	if err != nil {
		log.Println("param decode error, param coded:", picUrlcoded)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	picUrl := string(picUrldecoded)
	picUrl = strings.TrimRight(picUrl, string(0x0)) //rtrim tailing zero
	picId := getCacheId(picUrl)
	log.Println("requesting pic id:", picId)
	sessionId := req.Form.Get("sid")

	if !cacheExist(picId) {
		log.Println("cache miss, id:", picId)

		if err := makeCache(picUrl); err != nil {
			log.Println("make cache error, id:", picId, "error:", err)
			w.WriteHeader(http.StatusInternalServerError)
			failCountLock.Lock()
			picFailCount[sessionId]++
			failCountLock.Unlock()
			return
		}
	} else {
		log.Println("cache hit, pic id:", picId)
	}

	//读取缓存
	cacheContent, err = cacheRead(picId)
	if err != nil {
		log.Println("cache read error, id:", picId, "error:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(*cacheContent)
	log.Println("serve pic done, id:", picId)
	successCountLock.Lock()
	picSuccessCount[sessionId]++
	successCountLock.Unlock()
}

//任务预处理
func getPicJob(w http.ResponseWriter, req *http.Request) {
	req.ParseForm()
	for _, url := range req.PostForm["url"] {
		go func() {
			url = strings.TrimRight(url, string(0x0))
			picId := getCacheId(url)
			if !cacheExist(picId) {
				makeCache(url)
			}
		}()
	}
	w.Write([]byte("Job Received!"))
}

//失败计数
func fail2refresh(w http.ResponseWriter, req *http.Request) {
	req.ParseForm()
	sessionId := req.Form.Get("sid")
	for {
		if picFailCount[sessionId] > 5 {
			w.Write([]byte("window.location.reload();"))
			break
		} else if picSuccessCount[sessionId] > 5 {
			w.Write([]byte("/* normal */"))
			break
		}
		time.Sleep(time.Duration(50) * time.Millisecond)
		runtime.Gosched()
	}

	delete(picFailCount, sessionId)
}

//制作缓存
func makeCache(url string) (err error) {
	var respBody []byte
	var mcLock *sync.Mutex
	var ok bool
	picId := getCacheId(url)

	makingIdLock.RLock() //读写锁
	if mcLock, ok = makingId[picId]; !ok {
		makingIdLock.RUnlock()

		makingIdLock.Lock()
		mcLock = &sync.Mutex{}
		makingIdLock.Unlock()
	} else {
		makingIdLock.RUnlock()
	}
	mcLock.Lock()
	defer mcLock.Unlock()
	if cacheExist(picId) {
		//可能别的goroutine也在做这个id的cache,拿到锁正是别人做完的时候
		log.Println("make cache job done by other goroutine")
		return nil
	}

	log.Println("making cache, url:", url)
	//请求远端
	for i := 0; i < 2; i++ { //最大重试次数
		resp, err := http.Get(url)
		if err != nil {
			log.Println("request remote fail, exiting, url:", url)
			return err
		}
		if resp.StatusCode != 200 {
			if resp.StatusCode == 404 { //404 not retry
				log.Println("request remote fail, 404")
				break
			} else {
				log.Println("request remote fail, code:", resp.StatusCode, "waiting for retry")
				time.Sleep(time.Duration((i+1)*500) * time.Millisecond)
			}
		} else {
			respBody, _ = ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			log.Println("request remote done, size:", len(respBody), "id:", picId)
			break
		}
		resp.Body.Close()
	}

	if len(respBody) == 0 {
		return errors.New("makeCache empty response, url:" + url)
	}

	//存入缓存
	err = cacheWrite(picId, &respBody)
	if err != nil {
		log.Println("cache save fail, id:", picId)
		return err
	} else {
		log.Println("cache save succeed, id:", picId)
		return nil
	}
}
