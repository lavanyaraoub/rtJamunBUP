package restapi

import (
	"EtherCAT/helper"
	"EtherCAT/logger"
	"net/http"
	"os"
)

type API struct {
}

type TextResponse struct {
	Status   string `json:"status"`
	Response string `json:"response"`
}

func NewApi() API {
	return API{}
}

//Start start the rest api server
// for more details https://blog.logrocket.com/creating-a-web-server-with-golang/
func (a API) Start() {
	if _, err := os.Stat(helper.AppendWDPath("/gm_codes")); os.IsNotExist(err) {
		os.Mkdir(helper.AppendWDPath("/gm_codes"), 0777)
	}
	logger.Info("Rest service starting on port 5000")

	http.HandleFunc("/programs", getProgramFiles)
	http.HandleFunc("/getContents", getProgramFileContent)
	http.HandleFunc("/createFile", saveProgram)
	http.HandleFunc("/deleteFile", deleteProgram)
	http.HandleFunc("/renameFile", renameProgramFile)
	http.HandleFunc("/dac_params", manipulateSettings)
	http.HandleFunc("/faq", readFaq)
	http.HandleFunc("/support", getSupport)
	http.HandleFunc("/password", readPwd)
	http.HandleFunc("/hotspot/start", createHotspot)
	http.HandleFunc("/hotspot/stop", killHotspot)
	http.HandleFunc("/wifi/configure", configureWifi)
	//http.HandleFunc("/remote/start/http", startHTTPTunnel)
	http.HandleFunc("/remote/start/http", startHTTPTunnel)
	http.HandleFunc("/remote/stop", stopTunnel)
	//Code change for sending loggs over the air.
	//http.HandleFunc("/logs/send", sendLogs)
	// Expose files under /mnt/app/jamun via HTTP
http.Handle("/files/",
http.StripPrefix("/files/",
	http.FileServer(http.Dir("/mnt/app/jamun/"))))

	if err := http.ListenAndServe(":5000", nil); err != nil {
		logger.Fatal(err)
	}
}

func getCodeFilePath() string {
	return helper.GetCodeFilePath()
}

// setupCorsResponse if the request method is OPTIONS calls this method to setup the preflight response
func setupCorsResponse(w *http.ResponseWriter, req *http.Request) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
	(*w).Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
	(*w).Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
}

// usage http.Handle("/programs", corsHandler(getProgramFiles))
// but getProgramFiles is not a http.HandlerFunc
// func corsHandler(h http.Handler) http.HandlerFunc {
// 	return func(w http.ResponseWriter, r *http.Request) {
// 		if r.Method == "OPTIONS" {
// 			log.Print("preflight detected: ", r.Header)
// 			w.Header().Add("Connection", "keep-alive")
// 			w.Header().Add("Access-Control-Allow-Origin", "http://localhost")
// 			w.Header().Add("Access-Control-Allow-Methods", "POST, OPTIONS, GET, DELETE, PUT")
// 			w.Header().Add("Access-Control-Allow-Headers", "content-type")
// 			w.Header().Add("Access-Control-Max-Age", "86400")
// 		} else {
// 			h.ServeHTTP(w, r)
// 		}
// 	}
// }
