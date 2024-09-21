package base

import (
	"droneOS/internal/config"
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"net/http"
)

type Message struct {
	ID   int    `json:"id"`
	Type string `json:"type"`
	Data string `json:"data"`
}

func handler(w http.ResponseWriter, r *http.Request) {
	var m Message
	err := json.NewDecoder(r.Body).Decode(&m)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Debugf("%+v", m)

	values, err := callFunctionByName(m.Type, m.Data)
	if err != nil {
		log.Fatal(err)
	} else {
		// Convert reflect.Value to interface{} and collect in a slice
		var interfaces []interface{}
		for _, v := range values {
			interfaces = append(interfaces, v.Interface())
		}

		// Serialize the slice of interfaces to JSON
		data, err := json.Marshal(interfaces)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		_, err = w.Write(data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func Main(s *config.Config) {
	http.HandleFunc("/", handler)

	log.Infof("HTTP server listening on port %d", s.Base.Port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf("%s:%d", s.Base.Host, s.Base.Port), nil))
}
