package protocol

type Message struct {
	ID   int    `json:"Id"`
	Cmd  string `json:"Cmd"`
	Data string `json:"Data"`
}
