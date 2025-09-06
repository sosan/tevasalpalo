package update

import (
	"log"
	"time"
)

var VersionBuild string

const (
	LAYOUT_TIMESTAMP = "2006-01-02T15:04:05Z"
)

func GetVersionBuild() time.Time {
	currentVersionBuild := TransformStringDatetoTime(VersionBuild)
	return currentVersionBuild
}

func TransformStringDatetoTime(currentDate string) time.Time {
	currentDateTime, err := time.Parse(LAYOUT_TIMESTAMP, currentDate)
	if err != nil {
		currentDate = time.Now().Format(LAYOUT_TIMESTAMP)
		currentDateTime, err = time.Parse(LAYOUT_TIMESTAMP, currentDate)
		if err != nil {
			log.Fatalf("ERROR | No es posible conversion fechas")
		}
	}
	return currentDateTime

}

func SetVersionBuild(version string) {
	VersionBuild = version
}
