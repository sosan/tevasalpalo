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
	fileName := getFileName()
	return fmt.Sprintf("%s/%s", URI_DOWNLOAD, fileName)
}

func getFileName() string {
	ostype := getOsType()
	fileName := "licencias.exe"
	if ostype == "linux" {
		fileName = "licencias"
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


















// package update

// import (
// 	"log"

// 	"runtime"
// 	"github.com/mouuff/go-rocket-update/pkg/provider"
// 	"github.com/mouuff/go-rocket-update/pkg/updater"
// )

// const (
// 	URI_DOWNLOAD = `github.com/sosan/keyspaces_scrapper`//`https://github.com/sosan/keyspaces_scrapper/releases/download/latest`
// 	OSTIPO = "windows"
// )

// func AutoUpdate() (bool, bool) {
// 	needUpdate := false
// 	updateOk := false
	
// 	// uriDownload := getDownloadURI()
// 	err := doUpdate()
// 	if err != nil {
// 		log.Fatalf("ERROR | No es posible conectarse")
// 	}
// 	log.Printf("%s", OSTIPO)
// 	log.Fatalf("TERMINADO")
// 	return needUpdate, updateOk
// }

// func getFileName() string {
// 	ostype := getOsType()
// 	fileName := "licencias.exe.zip"
// 	if ostype == "linux" {
// 		fileName = "licencias.zip"
// 	}
// 	return fileName
// }

// func getOsType() string {
// 	return runtime.GOOS
// }

// func doUpdate() error {
// 	fileName := getFileName()
// 	upD := &updater.Updater{
// 		Provider: &provider.Github{
// 			RepositoryURL: URI_DOWNLOAD,
// 			ArchiveName:   fileName,
// 		},
// 		ExecutableName: fileName,
// 		Version:        "v0.0.0",
// 	}

// 	log.Println(upD.Version)
// 	statusCode, err := upD.Update(); 
// 	if err != nil {
// 		log.Println(err)
// 		log.Println(statusCode)
// 	}

// 	// body, statusCode := httpclient.GetRequestRaw(url, "")

//     // if statusCode != 200 {
// 	// 	log.Fatalf("ERROR | No es posible conectarse")
// 	// 	return nil
//     // }


//     // err := selfupdate.Apply(*body, selfupdate.Options{})
//     // if err != nil {
//     //     // error handling
// 	// 	log.Fatalf("ERROR | No es posible conectarse")
//     // }
// 	// defer (*body).Close()
//     return err
// }
