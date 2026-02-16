package drone

type Task struct {
	Name string // package
	Data any
}

type Output struct {
	Name string
	Main func(i any) error
}
