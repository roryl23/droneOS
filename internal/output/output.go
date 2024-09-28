package output

type Task struct {
	Name  string // plugin
	Input interface{}
}

type Output struct {
	Name string
	Main func(i interface{}) error
}
