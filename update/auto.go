package update

import (
	"fmt"
	"log"
	"runtime"
	"time"

	"github.com/minio/selfupdate"
)

const (
	URI_DOWNLOAD                    = `https://github.com/sosan/tevasalpalo/releases/download/latest`
	WINDOWS_FILENAME_REMOTE_VERSION = "portable.exe"
	LINUX_FILENAME_REMOTE_VERSION   = "portable"
)

func AutoUpdate() (bool, bool) {
	// updateOk := false
	Updated = false
	needUpdate := GetNeedUpdate()
	if !needUpdate {
		return needUpdate, Updated
	}

	log.Println("ACTUALIZANDO......")
	uriDownload := getDownloadURI()
	err := doUpdate(uriDownload)
	if err != nil {
		log.Print("ERROR | No es posible conectarse")
		return needUpdate, Updated
	}
	Updated = true
	log.Println("ACTUALIZADO!!")
	time.Sleep(3 * time.Second)
	// os.Exit(0)
	// dumy return
	return needUpdate, Updated
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
		defer (*body).Close()
		log.Print("ERROR | Status code es diferente a 200")
		return fmt.Errorf("status code es diferente a 200")
	}
	err := selfupdate.Apply(*body, selfupdate.Options{})
	if err != nil {
		// error handling
		rerr := selfupdate.RollbackError(err)
		if rerr != nil {
			defer (*body).Close()
			fmt.Printf("ERROR | NO SE HA PODIDO ACTUALIZAR: %v", rerr)
			return fmt.Errorf("no se ha podido actualizar: %v", rerr)
		}
	}
	defer (*body).Close()
	return err
}

var Updated = false

func ForceUpdate() error {
	Updated = false
	log.Println("ACTUALIZANDO......")
	uriDownload := getDownloadURI()
	err := doUpdate(uriDownload)
	if err != nil {
		log.Print("ERROR | No es posible actualizarse")
		return err
	}
	Updated = true
	log.Println("ACTUALIZADO!!")
	return err
}
