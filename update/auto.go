package update

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/minio/selfupdate"
)

const (
	URI_DOWNLOAD = `https://github.com/sosan/tevasalpalo/releases/download/latest`
	WINDOWS_FILENAME_REMOTE_VERSION =  "portable.exe"
	LINUX_FILENAME_REMOTE_VERSION =  "portable"
)

func AutoUpdate() (bool, bool) {
	updateOk := false
	needUpdate := getNeedUpdate()
	if !needUpdate {
		return needUpdate, updateOk
	}

	log.Println("ACTUALIZANDO......")
	uriDownload := getDownloadURI()
	err := doUpdate(uriDownload)
	if err != nil {
		log.Fatalf("ERROR | No es posible conectarse")
	}
	log.Println("ACTUALIZADO!!")
	time.Sleep(3 * time.Second)
	os.Exit(0)
	// dumy return
	return needUpdate, updateOk
}

func getDownloadURI() string {
	fileName := getFileNameRemoteVersion()
	return fmt.Sprintf("%s/%s", URI_DOWNLOAD, fileName)
}

func getFileNameRemoteVersion() string {
	ostype := getOsType()
	fileName := WINDOWS_FILENAME_REMOTE_VERSION //"date_build_portable.exe.txt"
	if ostype == "linux" {
		fileName = LINUX_FILENAME_REMOTE_VERSION //"date_build_portable.txt"
	}
	return fileName
}

func getOsType() string {
	return runtime.GOOS
}

func doUpdate(url string) error {
	body, statusCode := GetRequestRaw(url, "")

	if statusCode != 200 {
		log.Fatalf("ERROR | Status code es diferente a 200")
		return nil
	}
	err := selfupdate.Apply(*body, selfupdate.Options{})
	if err != nil {
		// error handling
		rerr := selfupdate.RollbackError(err)
		if rerr != nil {
			fmt.Printf("ERROR | NO SE HA PODIDO ACTUALIZAR: %v", rerr)
		}
	}
	defer (*body).Close()
	return err
}

func ForceUpdate() {
	log.Println("ACTUALIZANDO......")
	uriDownload := getDownloadURI()
	err := doUpdate(uriDownload)
	if err != nil {
		log.Fatalf("ERROR | No es posible conectarse")
	}
	log.Println("ACTUALIZADO!!")
	time.Sleep(3 * time.Second)
	os.Exit(0)
}