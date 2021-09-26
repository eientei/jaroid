package bot

import "sync"

type server struct {
	roles   map[string]map[string]struct{}
	members map[string]map[string]struct{}
	m       *sync.RWMutex
	prefix  string
}
