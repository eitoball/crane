package crane

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	uuid "github.com/hashicorp/go-uuid"
	"net/http"
	"runtime"
	"time"
)

type UpdateRequestParams struct {
	UUID    string `json:"uuid"`
	Arch    string `json:"arch"`
	OS      string `json:"os"`
	Version string `json:"version"`
	Pro     bool   `json:"pro"`
}

type UpdateResponseBody struct {
	Outdated              bool   `json:"outdated"`
	LatestVersion         string `json:"latest_version"`
	LatestReleaseDate     string `json:"latest_release_date"`
	LatestInstallationUrl string `json:"latest_installation_url"`
	LatestChangelogUrl    string `json:"latest_changelog_url"`
}

func checkForUpdates(manual bool) {
	client := &http.Client{Timeout: 3 * time.Second}
	uuid, _ := uuid.GenerateUUID()
	params := UpdateRequestParams{
		UUID:    uuid,
		Arch:    runtime.GOARCH,
		OS:      runtime.GOOS,
		Version: Version,
		Pro:     Pro,
	}
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(params)

	response := &UpdateResponseBody{}
	printInfof("Checking for updates ...\n")
	res, err := client.Post(
		"https://www.craneup.tech/update-checks",
		"application/json; charset=utf-8",
		b,
	)
	if err == nil && res.StatusCode != 200 {
		msg := fmt.Sprintf("Wrong status code %s", res.Status)
		err = errors.New(msg)
	}
	if err != nil {
		if manual {
			printErrorf("ERROR: %s\n", err)
		} else {
			verboseLog(err.Error())
		}
		return
	}

	defer res.Body.Close()

	json.NewDecoder(res.Body).Decode(response)

	if response.Outdated {
		printNoticef("Newer version %s is available!\n\n", response.LatestVersion)
		fmt.Printf("\tRelease Date: %s\n", response.LatestReleaseDate)
		fmt.Printf("\tChangelog: %s\n", response.LatestChangelogUrl)
		fmt.Printf("\nUpdate now: %s\n", response.LatestInstallationUrl)
	} else {
		printSuccessf("Version %s is up-to-date!\n", Version)
	}
}
