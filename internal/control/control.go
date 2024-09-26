package control

import (
	"container/list"
	"droneOS/internal/config"
	"droneOS/internal/input/sensor"
	"droneOS/internal/output"
	"fmt"
	log "github.com/sirupsen/logrus"
	"path/filepath"
	"plugin"
	"sync"
)

type PriorityMutex struct {
	mu        sync.Mutex
	isLocked  bool
	waitQueue map[int]*list.List // Priority level to list of channels
}

func NewPriorityMutex() *PriorityMutex {
	return &PriorityMutex{
		waitQueue: make(map[int]*list.List),
	}
}

func (pm *PriorityMutex) Lock(priority int) {
	pm.mu.Lock()
	if !pm.isLocked {
		pm.isLocked = true
		pm.mu.Unlock()
		return
	}

	// Create a channel for this goroutine to wait on
	ch := make(chan struct{})
	if pm.waitQueue[priority] == nil {
		pm.waitQueue[priority] = list.New()
	}
	// Add the channel to the wait queue for the given priority
	pm.waitQueue[priority].PushBack(ch)
	pm.mu.Unlock()

	// Wait on the channel
	<-ch
}

func (pm *PriorityMutex) Unlock() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Release the lock
	pm.isLocked = false

	// Find the highest priority level with waiting goroutines
	for priority := 1; priority <= 10; priority++ {
		if queue, ok := pm.waitQueue[priority]; ok && queue.Len() > 0 {
			// Remove the first goroutine in the queue
			elem := queue.Front()
			ch := elem.Value.(chan struct{})
			queue.Remove(elem)
			if queue.Len() == 0 {
				delete(pm.waitQueue, priority)
			}
			// Lock is now held by the waiting goroutine
			pm.isLocked = true
			// Signal the waiting goroutine
			close(ch)
			return
		}
	}
}

type Algorithm struct {
	Name string
	Main func(c *config.Config, priority int, cL *PriorityMutex, sCh *chan sensor.Event, tq chan output.Task)
}

func LoadPlugins(c *config.Config) []Algorithm {
	pluginDir := "./"

	// Find all .so files in the directory
	pluginFiles, err := filepath.Glob(filepath.Join(pluginDir, "*so"))
	if err != nil {
		log.Fatalf("Error finding plugin files: %v", err)
	}

	// Load functions in the configured priority order
	functions := make([]Algorithm, 0)
	for _, pluginName := range c.Drone.ControlAlgorithmPriority {
		for _, pluginFile := range pluginFiles {
			if pluginFile == fmt.Sprintf("%s_so", pluginName) {
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
				mainFunc, ok := symMain.(func(c *config.Config, p int, cL *PriorityMutex, sCh *chan sensor.Event, tq chan output.Task))
				if !ok {
					log.Fatalf("Main function in %s has incorrect signature\n", pluginFile)
					continue
				}
				functions = append(functions, Algorithm{Name: pluginName, Main: mainFunc})
			}
		}
	}
	return functions
}
