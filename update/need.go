package update

import (
	"fmt"
	"strings"
	"time"
)

func getNeedUpdate() bool {
	selfVersion := GetVersionBuild()
	remoteVersion := getRemoteVersionBuild()

	return remoteVersion.After(selfVersion)
}

func getRemoteVersionBuild() time.Time {
	uri := getRemotePathBuildDate()
	versionDate, statusCode := GetRequest(uri, "")
	if statusCode != 200 {
		versionDate = []byte("")
	}
	strVersion := cleanRawDate(string(versionDate))
	remoteVersionBuild := TransformStringDatetoTime(strVersion)
	return remoteVersionBuild
}

func cleanRawDate(dateRaw string) string {
	splitted := strings.Split(dateRaw, "\n")
	if len(splitted) == 0 {
		return ""
	}
	return splitted[0]
}

func getRemotePathBuildDate() string {
	fileName := getFileName()
	return fmt.Sprintf("%s/date_build_%s.txt", URI_DOWNLOAD, fileName)
}
