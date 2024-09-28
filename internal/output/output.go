package output

type Task struct {
	Name string // package
	Data interface{}
}

type Output struct {
	Name string
	Main func(i interface{}) error
}
