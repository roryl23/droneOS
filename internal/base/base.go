package base

import (
	"droneOS/internal/config"
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"net/http"
)

type Message struct {
	Text string `json:"text"`
	ID   int    `json:"id"`
}

func handler(w http.ResponseWriter, r *http.Request) {
	var msg Message
	err := json.NewDecoder(r.Body).Decode(&msg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Infof("Received message: %+v\n", msg)
	w.WriteHeader(http.StatusOK)
}

func Main(s *config.Config) {
	http.HandleFunc("/", handler)

	log.Infof("HTTP server listening on port %d ...", s.Base.Port)
	http.ListenAndServe(fmt.Sprintf("%s:%d", s.Base.Host, s.Base.Port), nil)
}
