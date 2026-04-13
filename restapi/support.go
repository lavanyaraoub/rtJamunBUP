package restapi

import (
	"EtherCAT/constants"
	"EtherCAT/helper"
	"encoding/json"
	"io/ioutil"
	"net/http"
)

type Support struct {
	Sales   Contact `json:"sales"`
	Service Contact `json:"service"`
	Version string  `json:"version"`
}

type Contact struct {
	ContactNumber string `json:"contact"`
	Email         string `json:"email"`
}

func getSupport(w http.ResponseWriter, r *http.Request) {
	setupCorsResponse(&w, r)
	if (*r).Method == "OPTIONS" {
		return
	}
	if (*r).Method == "GET" {
		suprt, _ := parseSupport()
		suprt.Version = constants.SystemVersion
		// resp := SettingsResponse{Status: "success", Response: json.RawMessage(settings)}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(suprt)
	}
}

func parseSupport() (Support, error) {
	var suprt Support
	path := helper.AppendWDPath("/configs/support.json")
	aboutFile, err := ioutil.ReadFile(path)
	if err != nil {
		return suprt, err
	}
	err = json.Unmarshal(aboutFile, &suprt)
	return suprt, err
}
