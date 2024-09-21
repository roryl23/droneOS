package protocol

func Ping(m Message) Message {
	return Message{
		ID:   m.ID,
		Cmd:  m.Cmd,
		Data: "pong",
	}
}
