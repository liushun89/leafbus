package playback

import (
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type synchronizer struct {
	controlChanel chan int
	syncChannels  map[int64][]chan *time.Time
	syncMtx       sync.Mutex
}

func NewSynchroinzer(controlChanel chan int) *synchronizer {
	return &synchronizer{
		controlChanel: controlChanel,
		syncChannels:  map[int64][]chan *time.Time{},
		syncMtx:       sync.Mutex{},
	}
}

func (s *synchronizer) addSyncChannel(id int64) chan *time.Time {
	s.syncMtx.Lock()
	defer s.syncMtx.Unlock()
	c := make(chan *time.Time, 10)
	if _, ok := s.syncChannels[id]; ok {
		s.syncChannels[id] = append(s.syncChannels[id], c)
	} else {
		s.syncChannels[id] = []chan *time.Time{c}
	}
	return c
}

func (s *synchronizer) removeSyncChannel(id int64, c chan *time.Time) {
	s.syncMtx.Lock()
	defer s.syncMtx.Unlock()
	if e, ok := s.syncChannels[id]; ok {
		idx := -1
		for i := range e {
			if e[i] == c {
				idx = i
				break
			}
		}
		if idx < 0 {
			return
		}
		e[idx] = e[len(e)-1]
		e[len(e)-1] = nil
		e = e[:len(e)-1]
	}
}

func (s *synchronizer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}
	run := r.Form.Get("run")
	start, end, err := bounds(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}
	if strings.ToLower(run) == "true" {
		log.Println("Starting Services from HTTP Request")
		s.run(start, end, 5)
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusBadRequest)
}

func (s *synchronizer) run(start, end time.Time, scale int64) {
	next := start
	lastSent := time.Unix(0, 0)
	ticker := time.NewTicker(1 * time.Millisecond)
	for {
		select {
		case <-ticker.C:
			time.Sleep(1 * time.Millisecond)
			now := time.Now()
			elapsed := now.Sub(lastSent).Milliseconds()
			// Default 10ms tick for timestamps
			if elapsed < 10/scale {
				continue
			}
			lastSent = now
			// Send time to channels
			s.syncMtx.Lock()
			for _, cs := range s.syncChannels {
				for _, c := range cs {
					if len(c) < 10 {
						c <- &next
					} else {
						log.Println("A sender is not keeping up, TS dropped")
					}
				}
			}
			s.syncMtx.Unlock()
			// Add the fixed 10ms to the counter
			next = next.Add(10 * time.Millisecond)
			if next.After(end) {
				log.Println("Reached end timestamp")
				break
			}
		}
	}
}
