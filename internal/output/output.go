package output

import (
	"container/heap"
	"droneOS/internal/config"
	"fmt"
	log "github.com/sirupsen/logrus"
	"path/filepath"
	"plugin"
	"time"
)

type Task struct {
	Priority int    // lower value means higher priority
	Index    int    // the index of the item in the heap
	Name     string // plugin
	Input    interface{}
}

// Queue implements heap.Interface and holds Tasks.
type Queue []*Task

func (pq Queue) Len() int { return len(pq) }

// Less determines the priority (higher priority comes first)
func (pq Queue) Less(i, j int) bool {
	return pq[i].Priority < pq[j].Priority
}

func (pq Queue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].Index = i
	pq[j].Index = j
}

// Push adds a Task to the heap
func (pq *Queue) Push(x interface{}) {
	n := len(*pq)
	task := x.(*Task)
	task.Index = n
	*pq = append(*pq, task)
}

// Pop removes and returns the highest priority Task
func (pq *Queue) Pop() interface{} {
	old := *pq
	n := len(old)
	task := old[n-1]
	old[n-1] = nil  // avoid memory leak
	task.Index = -1 // for safety
	*pq = old[0 : n-1]
	return task
}

type Output struct {
	Name string
	Main func(i interface{}) error
}

func LoadPlugins(c *config.Config) []Output {
	pluginDir := "./"

	// Find all .so files in the directory
	pluginFiles, err := filepath.Glob(filepath.Join(pluginDir, "output_*so"))
	if err != nil {
		log.Fatalf("Error finding plugin files: %v", err)
	}

	// Load functions in the configured priority order
	functions := make([]Output, 0)
	for _, device := range c.Drone.Outputs {
		for _, pluginFile := range pluginFiles {
			if pluginFile == fmt.Sprintf("%s_so", device.Name) {
				p, err := plugin.Open(pluginFile)
				if err != nil {
					log.Fatalf("Error loading plugin %s: %v\n", pluginFile, err)
					continue
				}
				// Look up the Main function
				symMain, err := p.Lookup("Main")
				if err != nil {
					log.Fatalf("Main function not found in %s: %v\n", pluginFile, err)
					continue
				}
				// Assert that loaded symbol is a function with the correct signature
				mainFunc, ok := symMain.(func(i interface{}) error)
				if !ok {
					log.Fatalf("Main function in %s has incorrect signature\n", pluginFile)
					continue
				}
				functions = append(functions, Output{Name: device.Name, Main: mainFunc})
			}
		}
	}
	return functions
}

func Main(pq Queue, plugins []Output) {
	for {
		for pq.Len() > 0 {
			task := heap.Pop(&pq).(*Task)
			for _, output := range plugins {
				if output.Name == task.Name {
					err := output.Main(task.Input)
					if err != nil {
						log.Error(err)
					}
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}
