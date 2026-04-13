package restapi

import (
	"EtherCAT/executors"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
)

//NewProgram holds the code for the new program
type NewProgram struct {
	FileName string `json:"file_name"`
	Code     string `json:"contents"`
}

func saveProgram(w http.ResponseWriter, r *http.Request) {
	setupCorsResponse(&w, r)
	if (*r).Method == "OPTIONS" {
		return
	}
	var program NewProgram
	err := json.NewDecoder(r.Body).Decode(&program)
	if err != nil {
		http.Error(w, "unable to save the program", http.StatusNotFound)
		return
	}

	content := []byte(program.Code)
	fileName := getCodeFilePath() + "/" + program.FileName
	err = ioutil.WriteFile(fileName, content, 0777)
	if err == nil {
		err = executors.CompileProgram(fileName)
	}
	if err != nil {
		os.Remove(fileName)
		json.NewEncoder(w).Encode(fmt.Sprintf("{\"status\":\"error\", \"desc\":\"%s\"}", err.Error()))
	} else {
		json.NewEncoder(w).Encode("{\"status\":\"success\"}")
	}
}
