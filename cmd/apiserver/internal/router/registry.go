package router

import (
	"log"
	"sync"

	"github.com/fsnotify/fsnotify"
)

type Registry struct {
	mu       sync.RWMutex
	router   *TrieRouter
	trie     *TrieRouter
	config   *RouteFile
	watcher  *fsnotify.Watcher
	reloadFn func(*RouteFile)
}

func NewRegistry(reloadFn func(*RouteFile)) *Registry {
	return &Registry{
		router:   NewTrieRouter(),
		trie:     NewTrieRouter(),
		reloadFn: reloadFn,
	}
}

func (r *Registry) Load(config *RouteFile) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.config = config
	r.trie = NewTrieRouter()
	for i := range config.Routes {
		r.trie.Insert(&config.Routes[i])
	}
	r.router = r.trie
	log.Printf("[router] loaded %d routes", len(config.Routes))
}

func (r *Registry) Match(method, path string) *MatchedRoute {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.router.Match(method, path)
}

func (r *Registry) Watch(path string) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("[router] watch error: %v", err)
		return
	}
	r.watcher = w

	go func() {
		for {
			select {
			case event, ok := <-w.Events:
				if !ok {
					return
				}
				if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
					log.Printf("[router] config file changed: %s", event.Name)
					if r.reloadFn != nil {
						r.reloadFn(r.config)
					}
				}
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				log.Printf("[router] watch error: %v", err)
			}
		}
	}()

	w.Add(path)
	log.Printf("[router] watching %s", path)
}

func (r *Registry) Config() *RouteFile {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.config
}

func (r *Registry) Routes() []RouteConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.config == nil {
		return nil
	}
	return r.config.Routes
}

func (r *Registry) Close() {
	if r.watcher != nil {
		r.watcher.Close()
	}
}
