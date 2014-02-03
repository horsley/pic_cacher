// cache
package main

import (
	"crypto/sha1"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

//sha1
func getCacheId(picUrl string) string {
	h := sha1.New()
	io.WriteString(h, picUrl)
	return fmt.Sprintf("%x", h.Sum(nil))
}

func getCacheFilename(cacheId string) string {
	return filepath.Join(CACHE_DIR, cacheId[0:2], cacheId[len(cacheId)-2:], cacheId)
}

//检查是否有缓存
func cacheExist(cacheId string) bool {
	_, err := os.Stat(getCacheFilename(cacheId))
	return !os.IsNotExist(err)
}

//缓存写入
func cacheWrite(cacheId string, data *[]byte) (err error) {
	filename := getCacheFilename(cacheId)
	os.MkdirAll(filepath.Dir(filename), 0777) //尝试创建目录
	err = ioutil.WriteFile(filename, *data, 0666)
	return
}

//缓存读取
func cacheRead(cacheId string) (data *[]byte, err error) {
	filename := getCacheFilename(cacheId)
	content, err := ioutil.ReadFile(filename)
	data = &content
	return
}
